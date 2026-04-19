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

// resolveHandles fetches all team feeds and merges with ExtraHandles.
// Returns deduped list of GitHub handles.
func resolveHandles(ctx context.Context, src Sources) ([]string, error) {
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
		body, err := httpGet(ctx, feed.URL, "")
		if err != nil {
			return nil, fmt.Errorf("fetching team feed %q: %w", feed.Name, err)
		}
		var team madeTeamJSON
		if err := json.Unmarshal(body, &team); err != nil {
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
	reBehaviors  = regexp.MustCompile(`^behaviors/([^/]+)\.yaml$`)
	reAgents     = regexp.MustCompile(`^agents/([^/]+)\.md$`)
	reRecipes    = regexp.MustCompile(`^recipes/([^/]+)\.yaml$`)
	reAmpRecipes = regexp.MustCompile(`^\.amplifier/recipes/([^/]+)\.yaml$`)
	rePySection  = regexp.MustCompile(`^\[([^\]]+)\]`)
	rePyName     = regexp.MustCompile(`^name\s*=\s*"([^"]*)"`)
	rePyDesc     = regexp.MustCompile(`^description\s*=\s*"([^"]*)"`)
)

// ── token resolution ──────────────────────────────────────────────────────────

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

// rawFile fetches a file via raw.githubusercontent.com (no API quota).
func rawFile(owner, repo, branch, path string) (string, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s",
		owner, repo, branch, path)
	resp, err := http.Get(url) //nolint:noctx
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
func listReposByHandle(ctx context.Context, token, handle string, delay *time.Duration, remaining *int, resetAt *int64) ([]map[string]any, error) {
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
		body, status, err := ghGet(ctx, token, nextURL, delay, remaining, resetAt)
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

// listOwnRepos returns all repos the authenticated user can access (including private).
// Used only for ExtraHandles that match the token owner.
func listOwnRepos(ctx context.Context, token string, delay *time.Duration, remaining *int, resetAt *int64) ([]map[string]any, error) {
	var all []map[string]any
	nextPath := "user/repos?per_page=100&sort=pushed&affiliation=owner,collaborator,organization_member"

	for nextPath != "" {
		body, status, err := ghGet(ctx, token, nextPath, delay, remaining, resetAt)
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

func ExtractCapabilities(owner, repo, branch string, paths []string, readmeText string) ([]Capability, error) {
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
		content, err := rawFile(owner, repo, branch, fname)
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
		content, err := rawFile(owner, repo, branch, p)
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
		content, err := rawFile(owner, repo, branch, p)
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
		content, err := rawFile(owner, repo, branch, p)
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
		content, err := rawFile(owner, repo, branch, "pyproject.toml")
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

	var (
		delay     time.Duration
		remaining = 5000
		resetAt   int64
	)
	if token != "" {
		delay = 50 * time.Millisecond
	} else {
		delay = 1200 * time.Millisecond
	}

	// ── Resolve handles from team feeds ───────────────────────────────────────
	if !opts.Quiet {
		fmt.Print("Resolving team handles... ")
	}
	handles, err := resolveHandles(ctx, *src)
	if err != nil {
		return nil, err
	}
	if !opts.Quiet {
		fmt.Printf("%d handles from team feeds\n", len(handles))
	}

	// ── Collect repos from all handles ────────────────────────────────────────
	// Deduplicate by full_name across handles
	reposByKey := map[string]map[string]any{}

	// Own repos first (includes private)
	if !opts.Quiet {
		fmt.Print("Fetching your repos (including private)... ")
	}
	ownRepos, err := listOwnRepos(ctx, token, &delay, &remaining, &resetAt)
	if err != nil && !opts.Quiet {
		fmt.Printf("warning: %v\n", err)
	}
	for _, r := range ownRepos {
		if k, ok := r["full_name"].(string); ok && k != "" {
			reposByKey[k] = r
		}
	}
	if !opts.Quiet {
		fmt.Printf("%d repos\n", len(ownRepos))
	}

	// Team member repos (public only, but that's fine for teammates)
	for _, handle := range handles {
		if !opts.Quiet {
			fmt.Printf("  %s... ", handle)
		}
		repos, err := listReposByHandle(ctx, token, handle, &delay, &remaining, &resetAt)
		if err != nil {
			if !opts.Quiet {
				fmt.Printf("error: %v\n", err)
			}
			continue
		}
		for _, r := range repos {
			if k, ok := r["full_name"].(string); ok && k != "" {
				if _, exists := reposByKey[k]; !exists {
					reposByKey[k] = r
				}
			}
		}
		if !opts.Quiet {
			fmt.Printf("%d repos\n", len(repos))
		}
	}

	// Extra specific repos (fetched directly)
	for _, extraKey := range src.ExtraRepos {
		if _, exists := reposByKey[extraKey]; exists {
			continue
		}
		body, status, err := ghGet(ctx, token, "repos/"+extraKey, &delay, &remaining, &resetAt)
		if err != nil || status != 200 {
			continue
		}
		var r map[string]any
		if json.Unmarshal(body, &r) == nil {
			reposByKey[extraKey] = r
		}
	}

	if !opts.Quiet {
		fmt.Printf("\nScanning %d unique repos for amplifier bundles...\n", len(reposByKey))
	}

	result := &ScanResult{}

	// Track which keys we saw (for removal detection)
	seenKeys := map[string]bool{}
	for k := range reposByKey {
		seenKeys[k] = true
	}

	for key, repo := range reposByKey {
		archived, _ := repo["archived"].(bool)
		if archived && !opts.IncludeArchived {
			result.Skipped++
			continue
		}

		pushedAt, _ := repo["pushed_at"].(string)
		defaultBranch, _ := repo["default_branch"].(string)
		if defaultBranch == "" {
			defaultBranch = "main"
		}

		cached := st.Repos[key]

		// Gate 1: pushed_at unchanged
		if pushedAt != "" && cached.PushedAt == pushedAt && !opts.Force {
			if cached.AmplifierLike {
				result.Unchanged++
			} else {
				result.Skipped++
			}
			continue
		}

		// Fetch git tree
		treePath := fmt.Sprintf("repos/%s/git/trees/%s?recursive=1", key, defaultBranch)
		treeBody, status, err := ghGet(ctx, token, treePath, &delay, &remaining, &resetAt)
		if err != nil {
			result.Skipped++
			continue
		}
		switch status {
		case 409: // empty repo
			st.Repos[key] = RepoState{PushedAt: pushedAt, AmplifierLike: false}
			result.Skipped++
			continue
		case 403, 404:
			result.Skipped++
			continue
		}
		if status != 200 {
			result.Skipped++
			continue
		}

		var treeResp struct {
			SHA  string `json:"sha"`
			Tree []struct {
				Path string `json:"path"`
			} `json:"tree"`
		}
		if err := json.Unmarshal(treeBody, &treeResp); err != nil {
			result.Skipped++
			continue
		}
		treeSha := treeResp.SHA
		paths := make([]string, 0, len(treeResp.Tree))
		for _, t := range treeResp.Tree {
			paths = append(paths, t.Path)
		}

		// Gate 2: tree SHA unchanged
		if treeSha != "" && cached.TreeSha == treeSha && !opts.Force {
			st.Repos[key] = RepoState{
				PushedAt:      pushedAt,
				TreeSha:       cached.TreeSha,
				AmplifierLike: cached.AmplifierLike,
			}
			if cached.AmplifierLike {
				result.Unchanged++
			} else {
				result.Skipped++
			}
			continue
		}

		// Check amplifier signatures
		isAmplifier := treeIsAmplifierLike(paths)
		st.Repos[key] = RepoState{
			PushedAt:      pushedAt,
			TreeSha:       treeSha,
			AmplifierLike: isAmplifier,
		}

		if !isAmplifier {
			if _, existed := idx.Repos[key]; existed {
				result.Removed = append(result.Removed, key)
				delete(idx.Repos, key)
			} else {
				result.Skipped++
			}
			continue
		}

		// Extract capabilities
		parts := strings.SplitN(key, "/", 2)
		ownerName, repoName := parts[0], parts[1]
		readmeText, _ := rawFile(ownerName, repoName, defaultBranch, "README.md")
		caps, _ := ExtractCapabilities(ownerName, repoName, defaultBranch, paths, readmeText)

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

		_, existed := idx.Repos[key]
		idx.Repos[key] = entry

		if existed {
			result.Updated = append(result.Updated, entry)
			if !opts.Quiet {
				fmt.Printf("  ~ %s (updated)\n", key)
			}
		} else {
			result.Added = append(result.Added, entry)
			if !opts.Quiet {
				fmt.Printf("  + %s\n", key)
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
	st.RateLimit = &RateLimitInfo{Remaining: remaining, ResetAt: resetAt}
	result.APIRemaining = remaining

	if err := SaveIndex(dir, idx); err != nil {
		return nil, fmt.Errorf("saving index: %w", err)
	}
	if err := SaveState(dir, st); err != nil {
		return nil, fmt.Errorf("saving state: %w", err)
	}

	return result, nil
}
