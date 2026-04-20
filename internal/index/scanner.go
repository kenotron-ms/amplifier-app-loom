package index

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ── ScanOptions / ScanResult ──────────────────────────────────────────────────

type ScanOptions struct {
	Force           bool
	IncludeArchived bool
	Quiet           bool
}

type ScanResult struct {
	Added        []Entry
	Updated      []Entry
	Removed      []string
	Unchanged    int
	Skipped      int
	APIRemaining int
}

// ── Sources config ────────────────────────────────────────────────────────────
//
// ~/.amplifier/bundle-index/sources.json — NOT in any repo.
// Controls which repos get scanned.

type Sources struct {
	// Remote team-JSON feeds. Each URL should point to a JSON file with a
	// "team_members" array containing "gh_handle" fields (and optional "directs").
	// Handles are extracted and each handle's repos are scanned.
	TeamFeeds []TeamFeed `json:"team_feeds"`

	// Additional individual GitHub handles to scan.
	ExtraHandles []string `json:"extra_handles"`

	// Additional specific repos to always scan (org/repo format).
	ExtraRepos []string `json:"extra_repos"`
}

type TeamFeed struct {
	Name string `json:"name"`
	URL  string `json:"url"` // raw URL to a team JSON file
}

// madeTeamJSON is the expected schema of a team feed JSON file.
type madeTeamJSON struct {
	TeamMembers []madeTeamMember `json:"team_members"`
}

type madeTeamMember struct {
	Name     string           `json:"name"`
	GHHandle string           `json:"gh_handle"`
	Directs  []madeTeamMember `json:"directs"`
}

// extractHandles flattens all gh_handle values from the tree.
func extractHandles(members []madeTeamMember) []string {
	seen := map[string]bool{}
	var out []string
	var walk func(m madeTeamMember)
	walk = func(m madeTeamMember) {
		if m.GHHandle != "" && !seen[m.GHHandle] {
			seen[m.GHHandle] = true
			out = append(out, m.GHHandle)
		}
		for _, d := range m.Directs {
			walk(d)
		}
	}
	for _, m := range members {
		walk(m)
	}
	return out
}

// stripTrailingCommas removes trailing commas before ] or } to handle
// relaxed JSON files (e.g. those edited by humans without strict JSON linting).
func stripTrailingCommas(data []byte) []byte {
	// Replace ,<whitespace>} and ,<whitespace>] patterns
	result := trailingCommaRe.ReplaceAll(data, []byte(""))
	return result
}

// resolveHandles fetches all team feeds and merges with ExtraHandles.
// Returns deduped list of GitHub handles.
func resolveHandles(ctx context.Context, token string, src Sources) ([]string, error) {
	seen := map[string]bool{}
	var handles []string

	add := func(h string) {
		h = strings.TrimSpace(h)
		if h != "" && !seen[h] {
			seen[h] = true
			handles = append(handles, h)
		}
	}

	for _, feed := range src.TeamFeeds {
		body, err := httpGet(ctx, feed.URL, token)
		if err != nil {
			return nil, fmt.Errorf("fetching team feed %q: %w", feed.Name, err)
		}
		body = stripTrailingCommas(body)
		var team madeTeamJSON
		decoder := json.NewDecoder(strings.NewReader(string(body)))
		if err := decoder.Decode(&team); err != nil {
			return nil, fmt.Errorf("parsing team feed %q: %w", feed.Name, err)
		}
		for _, h := range extractHandles(team.TeamMembers) {
			add(h)
		}
	}

	for _, h := range src.ExtraHandles {
		add(h)
	}

	return handles, nil
}

// ── compiled regexps ──────────────────────────────────────────────────────────

var (
	trailingCommaRe = regexp.MustCompile(`,\s*([}\]])`)
	reBehaviors  = regexp.MustCompile(`^behaviors/([^/]+)\.yaml$`)
	reAgents     = regexp.MustCompile(`^agents/([^/]+)\.md$`)
	reRecipes    = regexp.MustCompile(`^recipes/([^/]+)\.yaml$`)
	reAmpRecipes = regexp.MustCompile(`^\.amplifier/recipes/([^/]+)\.yaml$`)
	rePySection  = regexp.MustCompile(`^\[([^\]]+)\]`)
	rePyName     = regexp.MustCompile(`^name\s*=\s*"([^"]*)"`)
	rePyDesc     = regexp.MustCompile(`^description\s*=\s*"([^"]*)"`)
)

// ── rate limiter ──────────────────────────────────────────────────────────────
// rateLimiter serialises GitHub API calls so concurrent workers don't race
// on the delay/remaining/resetAt state.

type rateLimiter struct {
	mu        sync.Mutex
	delay     time.Duration
	remaining int
	resetAt   int64
}

func newRateLimiter(token string) *rateLimiter {
	rl := &rateLimiter{remaining: 5000}
	if token != "" {
		rl.delay = 50 * time.Millisecond
	} else {
		rl.delay = 1200 * time.Millisecond
	}
	return rl
}

// call executes ghGet under the rate limiter lock.
func (rl *rateLimiter) call(ctx context.Context, token, path string) ([]byte, int, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return ghGet(ctx, token, path, &rl.delay, &rl.remaining, &rl.resetAt)
}

// ── token resolution ────────────────────────────────────────────────────────────

func resolveToken() string {
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		return tok
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err == nil {
		if tok := strings.TrimSpace(string(out)); tok != "" {
			return tok
		}
	}
	return ""
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

// httpGet is a plain GET used for non-API URLs (team feeds, raw files).
func httpGet(ctx context.Context, url, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// ghGet makes a rate-aware GET to the GitHub API.
// Updates *delay/*remaining/*resetAt from response headers.
// Returns (nil, 304, nil) for 304 Not Modified.
func ghGet(ctx context.Context, token, path string, delay *time.Duration, remaining *int, resetAt *int64) ([]byte, int, error) {
	if *delay > 0 {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-time.After(*delay):
		}
	}

	url := "https://api.github.com/" + strings.TrimPrefix(path, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if rem := resp.Header.Get("X-RateLimit-Remaining"); rem != "" {
		if v, e := strconv.Atoi(rem); e == nil {
			*remaining = v
		}
	}
	if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
		if v, e := strconv.ParseInt(reset, 10, 64); e == nil {
			*resetAt = v
		}
	}

	// Adjust delay for next call
	if token != "" {
		switch {
		case *remaining < 20:
			now := time.Now().Unix()
			if *resetAt > now {
				*delay = time.Duration(*resetAt-now+5) * time.Second
			} else {
				*delay = 50 * time.Millisecond
			}
		case *remaining < 200:
			*delay = 500 * time.Millisecond
		default:
			*delay = 50 * time.Millisecond
		}
	} else {
		*delay = 1200 * time.Millisecond
	}

	if resp.StatusCode == http.StatusNotModified {
		return nil, 304, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// rawFile fetches a file via raw.githubusercontent.com.
// Pass token for private repo access (sent as Authorization header).
// No API quota cost for public repos; private repos require authentication.
func rawFile(token, owner, repo, branch, path string) (string, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s",
		owner, repo, branch, path)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil
	}
	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

// parseNextLink extracts the "next" URL from a GitHub Link response header.
func parseNextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		segs := strings.SplitN(part, ";", 2)
		if len(segs) != 2 {
			continue
		}
		if strings.TrimSpace(segs[1]) == `rel="next"` {
			return strings.Trim(strings.TrimSpace(segs[0]), "<>")
		}
	}
	return ""
}

// listReposByHandle returns all repos for a given GitHub handle,
// following pagination.
func listReposByHandle(ctx context.Context, token, handle string, rl *rateLimiter) ([]map[string]any, error) {
	var all []map[string]any

	nextURL := fmt.Sprintf("user/repos?per_page=100&sort=pushed&affiliation=owner,collaborator,organization_member&type=all")
	// For other users' handles (not self), use /users/{handle}/repos
	// We always use /user/repos for our own handle since it includes private repos.
	// The caller will filter by handle's repos using the owner field.
	_ = handle // handled below via owner filter

	// Actually, the right call for "repos a specific person owns/contributes to":
	// /users/{handle}/repos gives public repos only.
	// /user/repos with affiliation gives everything you have access to, filtered by owner.
	// We use /user/repos and filter by owner == handle for private repos of your own,
	// plus /users/{handle}/repos for other team members' public repos.
	nextURL = fmt.Sprintf("users/%s/repos?per_page=100&sort=pushed&type=owner", handle)

	for nextURL != "" {
		body, status, err := rl.call(ctx, token, nextURL)
		if err != nil {
			return nil, err
		}
		if status == 404 {
			return nil, nil // handle doesn't exist
		}
		if status != 200 {
			return nil, fmt.Errorf("HTTP %d listing repos for %s", status, handle)
		}

		var page []map[string]any
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		all = append(all, page...)

		// We'd need the Link header, but ghGet doesn't return it.
		// For now, 100 repos per handle is sufficient for most team members.
		// TODO: thread Link header through ghGet if needed.
		break
	}
	return all, nil
}

// eventsResult holds the outcome of fetching events for one handle.
type eventsResult struct {
	repos    []string // full_names of repos with push/create activity since the cutoff
	overflow bool     // true if the 300-event cap was hit before reaching the cutoff
}

// fetchEventsSince returns repos that had push or create activity for handle since the
// cutoff time. GitHub caps event history at 300 events (3 pages × 100); if we exhaust
// that without reaching the cutoff, overflow=true and the caller should fall back to a
// full repo listing for this handle.
func fetchEventsSince(ctx context.Context, token, handle string, since time.Time, rl *rateLimiter) (eventsResult, error) {
	seen := map[string]bool{}
	var repos []string
	totalEvents := 0

	for page := 1; page <= 3; page++ {
		path := fmt.Sprintf("users/%s/events?per_page=100&page=%d", handle, page)
		body, status, err := rl.call(ctx, token, path)
		if err != nil {
			return eventsResult{}, err
		}
		if status == 404 {
			break
		}
		if status != 200 {
			return eventsResult{}, fmt.Errorf("HTTP %d fetching events for %s", status, handle)
		}

		var events []struct {
			Type      string    `json:"type"`
			CreatedAt time.Time `json:"created_at"`
			Repo      struct {
				Name string `json:"name"` // "owner/repo"
			} `json:"repo"`
		}
		if err := json.Unmarshal(body, &events); err != nil {
			return eventsResult{}, err
		}
		if len(events) == 0 {
			break
		}

		pastCutoff := false
		for _, e := range events {
			totalEvents++
			if !e.CreatedAt.After(since) {
				pastCutoff = true
				break
			}
			if e.Type == "PushEvent" || e.Type == "CreateEvent" {
				if !seen[e.Repo.Name] {
					seen[e.Repo.Name] = true
					repos = append(repos, e.Repo.Name)
				}
			}
		}

		if pastCutoff || len(events) < 100 {
			break
		}
	}

	overflow := totalEvents >= 300
	return eventsResult{repos: repos, overflow: overflow}, nil
}

// listOwnRepos returns all repos the authenticated user can access (including private).
// Used only for ExtraHandles that match the token owner.
func listOwnRepos(ctx context.Context, token string, rl *rateLimiter) ([]map[string]any, error) {
	var all []map[string]any
	nextPath := "user/repos?per_page=100&sort=pushed&affiliation=owner,collaborator,organization_member"

	for nextPath != "" {
		body, status, err := rl.call(ctx, token, nextPath)
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, fmt.Errorf("HTTP %d listing own repos", status)
		}

		var page []map[string]any
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		all = append(all, page...)
		break // one page is enough for now (100 most-recently-pushed)
	}
	return all, nil
}

// ── amplifier detection ───────────────────────────────────────────────────────

func treeIsAmplifierLike(paths []string) bool {
	tier2 := false
	for _, p := range paths {
		switch p {
		case "bundle.md", "bundle.yaml", "bundle.yml":
			return true
		}
		if strings.HasPrefix(p, ".amplifier/") {
			return true
		}
		if reBehaviors.MatchString(p) || reAgents.MatchString(p) {
			return true
		}
		if reRecipes.MatchString(p) {
			tier2 = true
		}
	}
	return tier2
}

// ── capability extraction ─────────────────────────────────────────────────────

func parseFrontmatter(content string) map[string]any {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return nil
	}
	rest := content[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil
	}
	var out map[string]any
	_ = yaml.Unmarshal([]byte(rest[:end]), &out)
	return out
}

func nestedStr(data map[string]any, keys ...string) string {
	var cur any = data
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[k]
	}
	s, _ := cur.(string)
	return s
}

func readmeDescription(readme string) string {
	for _, line := range strings.Split(readme, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "![") ||
			strings.HasPrefix(line, "[![") || strings.HasPrefix(line, "<!--") ||
			strings.HasPrefix(line, "---") {
			continue
		}
		runes := []rune(line)
		if len(runes) > 200 {
			runes = runes[:200]
		}
		return string(runes)
	}
	return ""
}

func parsePyprojectTOML(content string) (name, desc string) {
	inProject := false
	for _, line := range strings.Split(content, "\n") {
		if m := rePySection.FindStringSubmatch(line); m != nil {
			inProject = m[1] == "project"
			continue
		}
		if !inProject {
			continue
		}
		if name == "" {
			if m := rePyName.FindStringSubmatch(line); m != nil {
				name = m[1]
			}
		}
		if desc == "" {
			if m := rePyDesc.FindStringSubmatch(line); m != nil {
				desc = m[1]
			}
		}
		if name != "" && desc != "" {
			break
		}
	}
	return
}

func ExtractCapabilities(token, owner, repo, branch string, paths []string, readmeText string) ([]Capability, error) {
	pathSet := make(map[string]bool, len(paths))
	for _, p := range paths {
		pathSet[p] = true
	}

	var caps []Capability

	// Pass 1: root bundle file
	for _, fname := range []string{"bundle.md", "bundle.yaml", "bundle.yml"} {
		if !pathSet[fname] {
			continue
		}
		content, err := rawFile(token, owner, repo, branch, fname)
		if err != nil || content == "" {
			continue
		}
		var data map[string]any
		if strings.HasSuffix(fname, ".md") {
			data = parseFrontmatter(content)
		} else {
			_ = yaml.Unmarshal([]byte(content), &data)
		}
		name := nestedStr(data, "bundle", "name")
		if name == "" {
			break
		}
		desc := nestedStr(data, "bundle", "description")
		if desc == "" {
			desc = readmeDescription(readmeText)
		}
		caps = append(caps, Capability{
			Type:        "bundle",
			Name:        name,
			Description: desc,
			Version:     nestedStr(data, "bundle", "version"),
			SourceFile:  fname,
		})
		break
	}

	// Pass 2: behaviors/*.yaml
	for _, p := range paths {
		m := reBehaviors.FindStringSubmatch(p)
		if m == nil {
			continue
		}
		content, err := rawFile(token, owner, repo, branch, p)
		if err != nil || content == "" {
			continue
		}
		var data map[string]any
		_ = yaml.Unmarshal([]byte(content), &data)
		name := nestedStr(data, "bundle", "name")
		if name == "" {
			name = m[1]
		}
		caps = append(caps, Capability{
			Type:        "behavior",
			Name:        name,
			Description: nestedStr(data, "bundle", "description"),
			SourceFile:  p,
		})
	}

	// Pass 3: agents/*.md
	for _, p := range paths {
		m := reAgents.FindStringSubmatch(p)
		if m == nil {
			continue
		}
		content, err := rawFile(token, owner, repo, branch, p)
		if err != nil || content == "" {
			continue
		}
		data := parseFrontmatter(content)
		name := nestedStr(data, "meta", "name")
		if name == "" {
			name = m[1]
		}
		caps = append(caps, Capability{
			Type:        "agent",
			Name:        name,
			Description: nestedStr(data, "meta", "description"),
			SourceFile:  p,
		})
	}

	// Pass 4: recipes
	for _, p := range paths {
		var fallback string
		if m := reRecipes.FindStringSubmatch(p); m != nil {
			fallback = m[1]
		} else if m := reAmpRecipes.FindStringSubmatch(p); m != nil {
			fallback = m[1]
		} else {
			continue
		}
		content, err := rawFile(token, owner, repo, branch, p)
		if err != nil || content == "" {
			continue
		}
		var data map[string]any
		_ = yaml.Unmarshal([]byte(content), &data)
		name, _ := data["name"].(string)
		if name == "" {
			_ = fallback
			continue
		}
		_, hasSteps := data["steps"]
		_, hasStages := data["stages"]
		if !hasSteps && !hasStages {
			continue
		}
		caps = append(caps, Capability{
			Type:       "recipe",
			Name:       name,
			SourceFile: p,
		})
	}

	// Pass 5: pyproject.toml
	if pathSet["pyproject.toml"] {
		content, err := rawFile(token, owner, repo, branch, "pyproject.toml")
		if err == nil && content != "" {
			if pName, pDesc := parsePyprojectTOML(content); pName != "" {
				caps = append(caps, Capability{
					Type:        "package",
					Name:        pName,
					Description: pDesc,
					SourceFile:  "pyproject.toml",
				})
			}
		}
	}

	return caps, nil
}

// ── main scan ─────────────────────────────────────────────────────────────────

// Scan scans repos from sources defined in sources.json and updates the index.
func Scan(ctx context.Context, dir string, opts ScanOptions) (*ScanResult, error) {
	token := resolveToken()

	src, err := LoadSources(dir)
	if err != nil {
		return nil, fmt.Errorf("loading sources: %w", err)
	}
	if len(src.TeamFeeds) == 0 && len(src.ExtraHandles) == 0 && len(src.ExtraRepos) == 0 {
		return nil, fmt.Errorf(
			"no sources configured — run: loom index init\n" +
				"  (creates ~/.amplifier/bundle-index/sources.json)")
	}

	idx, err := LoadIndex(dir)
	if err != nil {
		return nil, fmt.Errorf("loading index: %w", err)
	}
	st, err := LoadState(dir)
	if err != nil {
		return nil, fmt.Errorf("loading state: %w", err)
	}

	// ── Rate limiter (shared by discovery and scan workers) ──────────────────
	rl := newRateLimiter(token)

	// ── Resolve handles from team feeds ───────────────────────────────────────
	if !opts.Quiet {
		fmt.Print("Resolving team handles... ")
	}
	handles, err := resolveHandles(ctx, token, *src)
	if err != nil {
		return nil, err
	}
	if !opts.Quiet {
		fmt.Printf("%d handles from team feeds\n", len(handles))
	}

	result := &ScanResult{}

	// seenKeys tracks all discovered repos for removal detection.
	// Written only by the discovery goroutine; safe to read after outCh is drained.
	seenKeys := map[string]bool{}

	// Incremental mode: use events to diff against last scan instead of listing all repos.
	// Falls back to full scan on first run, --force, or if the index is empty.
	var lastScan time.Time
	if idx.LastScan != "" {
		lastScan, _ = time.Parse(time.RFC3339, idx.LastScan)
	}
	incremental := !opts.Force && !lastScan.IsZero() && len(idx.Repos) > 0

	// ── Parallel repo scan ──────────────────────────────────────────────────────
	// Gate 1 (pushed_at) is checked without any API call — pure local state.
	// Gate 2 + extraction require one ghGet (tree) + raw file reads per repo.
	// Workers share a rateLimiter that serialises ghGet calls while letting
	// raw file reads (rawFile / ExtractCapabilities) run fully concurrently.

	const workers = 8

	type repoWork struct {
		key  string
		repo map[string]any
	}
	type repoOutcome struct {
		key     string
		action  string // "added", "updated", "removed", "unchanged", "skip"
		entry   *Entry
		state   *RepoState
		prevKey string // for "removed"
	}

	workCh := make(chan repoWork, 64)
	outCh  := make(chan repoOutcome, 64)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range workCh {
				key := w.key
				repo := w.repo

				archived, _ := repo["archived"].(bool)
				if archived && !opts.IncludeArchived {
					outCh <- repoOutcome{key: key, action: "skip"}
					continue
				}

				pushedAt, _ := repo["pushed_at"].(string)
				defaultBranch, _ := repo["default_branch"].(string)
				if defaultBranch == "" {
					defaultBranch = "main"
				}

				// Read cached state (safe — read-only at this point)
				cached := st.Repos[key]

				// Gate 1: pushed_at unchanged — no API call
				if pushedAt != "" && cached.PushedAt == pushedAt && !opts.Force {
					action := "skip"
					if cached.AmplifierLike {
						action = "unchanged"
					}
					outCh <- repoOutcome{key: key, action: action}
					continue
				}

				// Gate 2: fetch tree (serialised through rate limiter)
				treePath := fmt.Sprintf("repos/%s/git/trees/%s?recursive=1", key, defaultBranch)
				treeBody, status, err := rl.call(ctx, token, treePath)
				if err != nil {
					outCh <- repoOutcome{key: key, action: "skip"}
					continue
				}
				switch status {
				case 409, 403, 404:
					rs := RepoState{PushedAt: pushedAt, AmplifierLike: false}
					outCh <- repoOutcome{key: key, action: "skip", state: &rs}
					continue
				}
				if status != 200 {
					outCh <- repoOutcome{key: key, action: "skip"}
					continue
				}

				var treeResp struct {
					SHA  string `json:"sha"`
					Tree []struct {
						Path string `json:"path"`
					} `json:"tree"`
				}
				if err := json.Unmarshal(treeBody, &treeResp); err != nil {
					outCh <- repoOutcome{key: key, action: "skip"}
					continue
				}
				treeSha := treeResp.SHA
				paths := make([]string, 0, len(treeResp.Tree))
				for _, t := range treeResp.Tree {
					paths = append(paths, t.Path)
				}

				// Tree SHA gate
				if treeSha != "" && cached.TreeSha == treeSha && !opts.Force {
					rs := RepoState{PushedAt: pushedAt, TreeSha: cached.TreeSha, AmplifierLike: cached.AmplifierLike}
					action := "skip"
					if cached.AmplifierLike {
						action = "unchanged"
					}
					outCh <- repoOutcome{key: key, action: action, state: &rs}
					continue
				}

				// Amplifier detection
				isAmplifier := treeIsAmplifierLike(paths)
				newState := &RepoState{PushedAt: pushedAt, TreeSha: treeSha, AmplifierLike: isAmplifier}

				if !isAmplifier {
					outCh <- repoOutcome{key: key, action: "removed", state: newState}
					continue
				}

				// File reads — fully parallel, no shared state (rawFile is pure HTTP)
				parts := strings.SplitN(key, "/", 2)
				ownerName, repoName := parts[0], parts[1]
				readmeText, _ := rawFile(token, ownerName, repoName, defaultBranch, "README.md")
				caps, _ := ExtractCapabilities(token, ownerName, repoName, defaultBranch, paths, readmeText)

				name, _ := repo["name"].(string)
				desc, _ := repo["description"].(string)
				if desc == "" {
					desc = readmeDescription(readmeText)
				}
				stars, _ := repo["stargazers_count"].(float64)
				private, _ := repo["private"].(bool)
				topicsAny, _ := repo["topics"].([]any)
				topics := make([]string, 0, len(topicsAny))
				for _, t := range topicsAny {
					if s, ok := t.(string); ok {
						topics = append(topics, s)
					}
				}

				entry := Entry{
					Remote:        key,
					Name:          name,
					Description:   desc,
					DefaultBranch: defaultBranch,
					Stars:         int(stars),
					Private:       private,
					Topics:        topics,
					Install:       fmt.Sprintf("amplifier bundle add git+https://github.com/%s@%s", key, defaultBranch),
					Capabilities:  caps,
					ScannedAt:     time.Now().UTC().Format(time.RFC3339),
				}
				outCh <- repoOutcome{key: key, action: "upsert", entry: &entry, state: newState}
			}
		}()
	}

	// Close output channel once all workers finish
	go func() { wg.Wait(); close(outCh) }()

	// Discovery goroutine: feeds workCh as repos are found so scanning begins immediately.
	// In incremental mode it uses the Events API to find only changed repos; in full mode
	// it lists all repos per handle. seenKeys is safe to read after outCh is drained
	// (happens-before via wg.Wait+close(outCh)).
	go func() {
		defer close(workCh)

		seen := map[string]bool{}
		sendRepo := func(r map[string]any) {
			k, ok := r["full_name"].(string)
			if !ok || k == "" {
				return
			}
			if seen[k] {
				return
			}
			seen[k] = true
			seenKeys[k] = true
			workCh <- repoWork{key: k, repo: r}
		}

		// fetchRepoByKey fetches a single repo's metadata and sends it to workCh.
		fetchRepoByKey := func(key string) {
			if seen[key] {
				return
			}
			body, status, err := rl.call(ctx, token, "repos/"+key)
			if err != nil || status != 200 {
				return
			}
			var r map[string]any
			if json.Unmarshal(body, &r) == nil {
				sendRepo(r)
			}
		}

		if incremental {
			// Pre-seed seenKeys with all cached repos so removal detection doesn't
			// cull repos we didn't re-visit. Repos deleted from GitHub stay in the
			// index until the next full scan (--force).
			for k := range idx.Repos {
				seenKeys[k] = true
			}
			if !opts.Quiet {
				fmt.Printf("Incremental scan (last: %s)\n", idx.LastScan)
			}

			// Own repos: always check to catch new/removed private repos.
			if !opts.Quiet {
				fmt.Print("Fetching your repos (including private)... ")
			}
			ownRepos, err := listOwnRepos(ctx, token, rl)
			if err != nil && !opts.Quiet {
				fmt.Printf("warning: %v\n", err)
			}
			for _, r := range ownRepos {
				sendRepo(r)
			}
			if !opts.Quiet {
				fmt.Printf("%d repos\n", len(ownRepos))
			}

			// Team repos: events diff — only fetch metadata for repos that changed.
			for _, handle := range handles {
				ev, err := fetchEventsSince(ctx, token, handle, lastScan, rl)
				if err != nil {
					if !opts.Quiet {
						fmt.Printf("  %s... events error: %v\n", handle, err)
					}
					continue
				}
				if ev.overflow {
					// Hit the 300-event cap; fall back to full listing for this handle.
					if !opts.Quiet {
						fmt.Printf("  %s... (events overflow) ", handle)
					}
					repos, err := listReposByHandle(ctx, token, handle, rl)
					if err != nil {
						if !opts.Quiet {
							fmt.Printf("error: %v\n", err)
						}
						continue
					}
					for _, r := range repos {
						sendRepo(r)
					}
					if !opts.Quiet {
						fmt.Printf("%d repos\n", len(repos))
					}
				} else {
					if !opts.Quiet {
						fmt.Printf("  %s... %d changed\n", handle, len(ev.repos))
					}
					for _, key := range ev.repos {
						fetchRepoByKey(key)
					}
				}
			}
		} else {
			// Full scan: list all repos for every handle.
			if !opts.Quiet {
				fmt.Print("Fetching your repos (including private)... ")
			}
			ownRepos, err := listOwnRepos(ctx, token, rl)
			if err != nil && !opts.Quiet {
				fmt.Printf("warning: %v\n", err)
			}
			for _, r := range ownRepos {
				sendRepo(r)
			}
			if !opts.Quiet {
				fmt.Printf("%d repos\n", len(ownRepos))
			}

			for _, handle := range handles {
				if !opts.Quiet {
					fmt.Printf("  %s... ", handle)
				}
				repos, err := listReposByHandle(ctx, token, handle, rl)
				if err != nil {
					if !opts.Quiet {
						fmt.Printf("error: %v\n", err)
					}
					continue
				}
				for _, r := range repos {
					sendRepo(r)
				}
				if !opts.Quiet {
					fmt.Printf("%d repos\n", len(repos))
				}
			}
		}

		// Extra specific repos — always checked regardless of mode (small explicit list).
		for _, extraKey := range src.ExtraRepos {
			fetchRepoByKey(extraKey)
		}
	}()

	// Collect results (single goroutine — no mutex needed on idx/st/result)
	for out := range outCh {
		if out.state != nil {
			st.Repos[out.key] = *out.state
		}
		switch out.action {
		case "unchanged":
			result.Unchanged++
		case "skip":
			result.Skipped++
		case "removed":
			if _, existed := idx.Repos[out.key]; existed {
				result.Removed = append(result.Removed, out.key)
				delete(idx.Repos, out.key)
			} else {
				result.Skipped++
			}
		case "upsert":
			_, existed := idx.Repos[out.key]
			idx.Repos[out.key] = *out.entry
			if existed {
				result.Updated = append(result.Updated, *out.entry)
				if !opts.Quiet {
					fmt.Printf("  ~ %s (updated)\n", out.key)
				}
			} else {
				result.Added = append(result.Added, *out.entry)
				if !opts.Quiet {
					fmt.Printf("  + %s\n", out.key)
				}
			}
		}
	}

	// Remove repos no longer in any source
	for k := range idx.Repos {
		if !seenKeys[k] {
			result.Removed = append(result.Removed, k)
			delete(idx.Repos, k)
		}
	}

	idx.LastScan = time.Now().UTC().Format(time.RFC3339)
	idx.Version = 1
	st.Version = 1
	st.RateLimit = &RateLimitInfo{Remaining: rl.remaining, ResetAt: rl.resetAt}
	result.APIRemaining = rl.remaining

	if err := SaveIndex(dir, idx); err != nil {
		return nil, fmt.Errorf("saving index: %w", err)
	}
	if err := SaveState(dir, st); err != nil {
		return nil, fmt.Errorf("saving state: %w", err)
	}

	return result, nil
}
