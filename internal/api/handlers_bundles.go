package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ms/amplifier-app-loom/internal/amplifier"
	"github.com/ms/amplifier-app-loom/internal/config"
)

// ── Registry proxy ────────────────────────────────────────────────────────────

// registryURL is the public community registry. Override with AMPLIFIER_REGISTRY_URL
// to point at a local server during development (e.g. python3 -m http.server 8765).
var registryURL = func() string {
	if u := os.Getenv("AMPLIFIER_REGISTRY_URL"); u != "" {
		return u
	}
	return "https://raw.githubusercontent.com/kenotron-ms/amplifier-registry/main/bundles.json"
}()

var (
	registryCache    []json.RawMessage
	registryCacheAt  time.Time
	registryCacheMu  sync.Mutex
	registryCacheTTL = time.Hour
)

// GET /api/registry
func (s *Server) getRegistry(w http.ResponseWriter, r *http.Request) {
	registryCacheMu.Lock()
	defer registryCacheMu.Unlock()

	if registryCache != nil && time.Since(registryCacheAt) < registryCacheTTL {
		writeJSON(w, http.StatusOK, registryCache)
		return
	}
	resp, err := http.Get(registryURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to fetch registry: "+err.Error())
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to read registry")
		return
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(body, &entries); err != nil {
		writeError(w, http.StatusBadGateway, "registry not valid JSON")
		return
	}
	registryCache = entries
	registryCacheAt = time.Now()
	writeJSON(w, http.StatusOK, registryCache)
}

// ── App bundle management ─────────────────────────────────────────────────────
//
// Source of truth: ~/.amplifier/settings.yaml → bundle.app (list of spec URIs).
// Loom's config.AppBundles is a metadata cache (id, name) used only for display.
//
//   Adding:  amplifier bundle add --app <spec>     + store metadata in loom config
//   Removing: amplifier bundle remove <spec> --app + remove metadata from loom config
//   Toggle:  add/remove from bundle.app via CLI    + update Enabled in loom config
//   GET:     read bundle.app for real enabled state; merge with loom metadata

type addBundleRequest struct {
	ID          string `json:"id"`
	InstallSpec string `json:"installSpec"`
	Name        string `json:"name,omitempty"`
}

// GET /api/bundles
func (s *Server) listBundles(w http.ResponseWriter, r *http.Request) {
	bundles := s.cfg.AppBundles
	if bundles == nil {
		bundles = []config.AppBundle{}
	}

	// Reconcile Enabled state against ~/.amplifier/settings.yaml
	if appSpecs, err := amplifier.ReadAppBundles(); err == nil {
		inApp := make(map[string]bool, len(appSpecs))
		for _, sp := range appSpecs {
			inApp[strings.TrimSpace(sp)] = true
		}
		for i, b := range bundles {
			bundles[i].Enabled = inApp[strings.TrimSpace(b.InstallSpec)]
		}
	}

	writeJSON(w, http.StatusOK, bundles)
}

// POST /api/bundles
func (s *Server) addBundle(w http.ResponseWriter, r *http.Request) {
	var req addBundleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.InstallSpec = strings.TrimSpace(req.InstallSpec)
	req.ID = strings.TrimSpace(req.ID)
	if req.InstallSpec == "" {
		writeError(w, http.StatusBadRequest, "installSpec is required")
		return
	}
	if req.ID == "" {
		req.ID = req.InstallSpec
	}

	for _, b := range s.cfg.AppBundles {
		if b.ID == req.ID {
			writeError(w, http.StatusConflict, "bundle already installed")
			return
		}
	}

	// Register with amplifier as an app bundle
	if err := ampBundleAddApp(req.InstallSpec); err != nil {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("amplifier bundle add --app failed: %v\nMake sure `amplifier` is installed.", err))
		return
	}

	bundle := config.AppBundle{
		ID:          req.ID,
		InstallSpec: req.InstallSpec,
		Name:        req.Name,
		Enabled:     true,
	}
	s.cfg.AppBundles = append(s.cfg.AppBundles, bundle)
	if err := s.store.SaveConfig(r.Context(), s.cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}
	writeJSON(w, http.StatusCreated, bundle)
}

// DELETE /api/bundles/{id}
func (s *Server) removeBundle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var spec string
	var wasEnabled bool
	filtered := make([]config.AppBundle, 0, len(s.cfg.AppBundles))
	for _, b := range s.cfg.AppBundles {
		if b.ID != id {
			filtered = append(filtered, b)
		} else {
			spec = b.InstallSpec
			wasEnabled = b.Enabled
		}
	}
	if spec == "" {
		writeError(w, http.StatusNotFound, "bundle not found")
		return
	}

	if wasEnabled {
		ampBundleRemoveApp(spec) //nolint:errcheck
	}

	s.cfg.AppBundles = filtered
	if err := s.store.SaveConfig(r.Context(), s.cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/bundles/{id}/toggle
func (s *Server) toggleBundle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	idx := -1
	for i, b := range s.cfg.AppBundles {
		if b.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		writeError(w, http.StatusNotFound, "bundle not found")
		return
	}

	b := &s.cfg.AppBundles[idx]
	var cliErr error
	if b.Enabled {
		cliErr = ampBundleRemoveApp(b.InstallSpec)
		if cliErr == nil {
			b.Enabled = false
		}
	} else {
		cliErr = ampBundleAddApp(b.InstallSpec)
		if cliErr == nil {
			b.Enabled = true
		}
	}

	if cliErr != nil {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("amplifier bundle command failed: %v", cliErr))
		return
	}

	if err := s.store.SaveConfig(r.Context(), s.cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}
	writeJSON(w, http.StatusOK, *b)
}

// ── amplifier CLI helpers ─────────────────────────────────────────────────────

func ampBundleAddApp(spec string) error {
	return runAmpCmd("bundle", "add", "--app", spec)
}

func ampBundleRemoveApp(spec string) error {
	return runAmpCmd("bundle", "remove", spec, "--app")
}

// runAmpCmd runs the amplifier binary with the given arguments, searching
// common install locations because the daemon may have a stripped PATH.
func runAmpCmd(args ...string) error {
	bin := resolveAmplifier()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "TERM=dumb")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return err
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// resolveAmplifier finds the amplifier binary across common install locations.
func resolveAmplifier() string {
	if p, err := exec.LookPath("amplifier"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, ".local", "bin", "amplifier"),
		"/usr/local/bin/amplifier",
		"/opt/homebrew/bin/amplifier",
		filepath.Join(home, "go", "bin", "amplifier"),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "amplifier"
}

// ── Amplifier-native bundle state ────────────────────────────────────────────
// Drives off ~/.amplifier/settings.yaml (bundle.added / .active / .app) and
// ~/.amplifier/registry.json rather than loom's own config store.

// AmplifierBundleEntry is the shape returned by GET /api/amplifier/bundles.
type AmplifierBundleEntry struct {
	Name       string  `json:"name"`
	URI        string  `json:"uri"`
	Active     bool    `json:"active"`     // this bundle is bundle.active
	AppEnabled bool    `json:"appEnabled"` // URI is in bundle.app
	AppSpec    string  `json:"appSpec"`    // exact spec to add/remove from bundle.app
	Downloaded bool    `json:"downloaded"` // local_path present in registry.json
	Version    *string `json:"version"`    // from registry.json, null if unknown
}

// GET /api/amplifier/bundles
func (s *Server) listAmplifierBundles(w http.ResponseWriter, _ *http.Request) {
	state, err := amplifier.ReadGlobalBundleSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read settings: "+err.Error())
		return
	}

	regEntries, _ := amplifier.ReadBundleRegistry() // tolerate missing registry

	// Build an O(1) lookup set for bundle.app (keyed by trimmed URI).
	appSet := make(map[string]bool, len(state.App))
	for _, uri := range state.App {
		appSet[strings.TrimSpace(uri)] = true
	}

	// Track which app URIs were matched to a bundle.added entry.
	matchedAppURIs := make(map[string]bool)

	entries := make([]AmplifierBundleEntry, 0, len(state.Added)+len(state.App))
	for name, uri := range state.Added {
		appSpec := strings.TrimSpace(uri)
		inApp := appSet[appSpec]
		if inApp {
			matchedAppURIs[appSpec] = true
		}
		entry := AmplifierBundleEntry{
			Name:       name,
			URI:        uri,
			Active:     name == state.Active,
			AppEnabled: inApp,
			AppSpec:    appSpec,
		}
		if re, ok := regEntries[name]; ok {
			entry.Downloaded = re.Downloaded()
			entry.Version = re.Version
		}
		entries = append(entries, entry)
	}

	// Include bundle.app entries that have no corresponding bundle.added entry.
	// These are "orphan" overlays — direct behavior/spec file references.
	for _, appURI := range state.App {
		spec := strings.TrimSpace(appURI)
		if matchedAppURIs[spec] {
			continue
		}
		// Derive a display name from the URI (basename without extension).
		name := filepath.Base(spec)
		name = strings.TrimPrefix(name, "file://")
		for _, ext := range []string{".yaml", ".yml", ".md"} {
			name = strings.TrimSuffix(name, ext)
		}
		entries = append(entries, AmplifierBundleEntry{
			Name:       name,
			URI:        appURI,
			AppEnabled: true,
			AppSpec:    spec,
			Downloaded: true, // assume local file exists
		})
	}

	// Alphabetical only — preserve user's expected order, no reordering by state.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	writeJSON(w, http.StatusOK, entries)
}

// POST /api/amplifier/bundles/app — enable a spec as always-on overlay.
// Body: {"spec": "<uri>"}
func (s *Server) enableAmplifierBundleApp(w http.ResponseWriter, r *http.Request) {
	spec := appSpecFromRequest(r)
	if spec == "" {
		writeError(w, http.StatusBadRequest, "spec is required")
		return
	}
	if err := ampBundleAddApp(spec); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/amplifier/bundles/app — remove a spec from always-on overlays.
// Body: {"spec": "<uri>"}
func (s *Server) disableAmplifierBundleApp(w http.ResponseWriter, r *http.Request) {
	spec := appSpecFromRequest(r)
	if spec == "" {
		writeError(w, http.StatusBadRequest, "spec is required")
		return
	}
	if err := ampBundleRemoveApp(spec); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/amplifier/bundles/{name}/activate — set as the primary active bundle
func (s *Server) activateAmplifierBundle(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := amplifier.SetGlobalActive(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/amplifier/bundles/active — clear bundle.active (falls back to foundation)
func (s *Server) clearAmplifierActive(w http.ResponseWriter, _ *http.Request) {
	if err := amplifier.SetGlobalActive(""); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/amplifier/bundles/{name} — remove bundle entirely
func (s *Server) removeAmplifierBundle(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := runAmpCmd("bundle", "remove", name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// appSpecFromRequest reads the "spec" field from a JSON body or "spec" query param.
func appSpecFromRequest(r *http.Request) string {
	var body struct {
		Spec string `json:"spec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.Spec != "" {
		return strings.TrimSpace(body.Spec)
	}
	return strings.TrimSpace(r.URL.Query().Get("spec"))
}

// ── Private local registry ────────────────────────────────────────────────────
// Reads ~/.amplifier/bundle-index/index.json — maintained by the local-index CLI
// (registry/.github/scripts/local-index.mjs).  Returns an empty array (not an
// error) if the index has not been initialised yet.

// localCapability mirrors capabilities[] entries from local-index.mjs.
type localCapability struct {
	Type        string  `json:"type"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Version     *string `json:"version,omitempty"`
	SourceFile  string  `json:"sourceFile"`
	Inferred    bool    `json:"inferred,omitempty"`
}

// localRepoEntry mirrors repos{} values from local-index.mjs index.json.
type localRepoEntry struct {
	RepoPath     string            `json:"repoPath"`
	Name         string            `json:"name"`
	Remote       string            `json:"remote"` // "org/repo" or ""
	SHA          string            `json:"sha"`
	ScannedAt    string            `json:"scannedAt"`
	Capabilities []localCapability `json:"capabilities"`
}

// localIndexFile mirrors the top-level local-index.mjs index.json.
type localIndexFile struct {
	Version  int                       `json:"version"`
	LastScan string                    `json:"lastScan"`
	Repos    map[string]localRepoEntry `json:"repos"`
}

// capTypePriority controls which capability type becomes the "primary" for a repo.
var capTypePriority = map[string]int{
	"bundle": 0, "behavior": 1, "agent": 2,
	"recipe": 3, "package": 4, "tool": 5,
}

// localRepoToEntry transforms a local repo into the RegistryEntry shape the UI
// expects.  Returns nil if the repo has no capabilities.
func localRepoToEntry(repo localRepoEntry) json.RawMessage {
	if len(repo.Capabilities) == 0 {
		return nil
	}

	// Pick the most significant (lowest-priority-number) capability.
	primary := repo.Capabilities[0]
	for _, c := range repo.Capabilities[1:] {
		if capTypePriority[c.Type] < capTypePriority[primary.Type] {
			primary = c
		}
	}

	namespace, repoURL, install := "", "", ""
	if repo.Remote != "" {
		if idx := strings.Index(repo.Remote, "/"); idx > 0 {
			namespace = repo.Remote[:idx]
		}
		repoURL = "https://github.com/" + repo.Remote
		install = "amplifier bundle add git+https://github.com/" + repo.Remote + "@main"
	} else {
		repoURL = "file://" + repo.RepoPath
		install = "amplifier bundle add git+file://" + repo.RepoPath
	}

	// Use remote slug as ID; fall back to basename of local path.
	id := repo.Remote
	if id == "" {
		id = filepath.Base(repo.RepoPath)
	}

	description := ""
	if primary.Description != nil {
		description = *primary.Description
	}

	lastUpdated := ""
	if len(repo.ScannedAt) >= 10 {
		lastUpdated = repo.ScannedAt[:10]
	}

	entry := map[string]any{
		"id":           id,
		"name":         primary.Name,
		"namespace":    namespace,
		"description":  description,
		"type":         primary.Type,
		"category":     "dev",
		"author":       namespace,
		"repo":         repoURL,
		"install":      install,
		"rating":       nil,
		"tags":         []string{},
		"featured":     false,
		"lastUpdated":  lastUpdated,
		"private":      true,
		"localPath":    repo.RepoPath,
		"capabilities": repo.Capabilities,
	}

	b, err := json.Marshal(entry)
	if err != nil {
		return nil
	}
	return json.RawMessage(b)
}

// GET /api/local-registry
func (s *Server) getLocalRegistry(w http.ResponseWriter, r *http.Request) {
	home, err := os.UserHomeDir()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cannot determine home dir")
		return
	}

	// Prefer GitHub-based index (github-index.mjs) over local-filesystem index.
	var data []byte
	for _, rel := range []string{
		filepath.Join(".amplifier", "github-bundle-index", "index.json"),
		filepath.Join(".amplifier", "bundle-index", "index.json"),
	} {
		if b, e := os.ReadFile(filepath.Join(home, rel)); e == nil {
			data = b
			break
		}
	}
	if data == nil {
		// Neither index seeded yet — return empty, not an error.
		writeJSON(w, http.StatusOK, []json.RawMessage{})
		return
	}

	var idx localIndexFile
	if err := json.Unmarshal(data, &idx); err != nil {
		writeError(w, http.StatusInternalServerError, "malformed local bundle index")
		return
	}

	entries := make([]json.RawMessage, 0, len(idx.Repos))
	for _, repo := range idx.Repos {
		if e := localRepoToEntry(repo); e != nil {
			entries = append(entries, e)
		}
	}
	writeJSON(w, http.StatusOK, entries)
}
