package mirror

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// BrowserFetcher uses agent-browser to navigate to a URL with a persistent
// browser profile and extract data from the page. The connector's Prompt
// describes what to extract; it's passed to browser-operator or used as
// a JS eval expression depending on the connector configuration.
//
// This fetcher is the most powerful but also the heaviest — it launches a
// headless browser on each sync. Use CommandFetcher or HTTPFetcher when the
// data source has an API.
type BrowserFetcher struct {
	// ProfileDir is the base directory for browser profiles.
	// Defaults to ~/.loom/browser-profiles/
	ProfileDir string
	// Timeout for browser operations. Defaults to 60s.
	Timeout time.Duration
}

// NewBrowserFetcher returns a BrowserFetcher with sensible defaults.
func NewBrowserFetcher(profileDir string) *BrowserFetcher {
	if profileDir == "" {
		profileDir = "~/.loom/browser-profiles"
	}
	return &BrowserFetcher{
		ProfileDir: profileDir,
		Timeout:    60 * time.Second,
	}
}

// Fetch navigates to the connector's URL using agent-browser with a persistent
// profile (grouped by connector.Site) and extracts data based on the connector's
// Prompt. If the Prompt looks like JavaScript (contains "document." or starts
// with "JSON.stringify"), it's run as an eval expression. Otherwise, it's used
// as a snapshot + prompt for the agent to extract structured data.
func (f *BrowserFetcher) Fetch(conn *Connector) (*FetchResult, error) {
	if conn.URL == "" {
		return nil, fmt.Errorf("browser fetcher: connector %s has no URL configured", conn.ID)
	}

	site := conn.Site
	if site == "" {
		site = "default"
	}
	profilePath := fmt.Sprintf("%s/%s", f.ProfileDir, site)
	sessionName := fmt.Sprintf("mirror-%s", conn.ID)

	// Build the agent-browser command sequence:
	// 1. Open the URL with persistent profile
	// 2. Wait for page load
	// 3. Take a snapshot (accessibility tree) for the agent to parse
	// The Prompt determines what to extract
	// Shared profile/session flags reused across all three commands so they
	// operate on the same persistent browser session.
	sessionFlags := []string{"--profile", profilePath, "--session-name", sessionName}

	// Step 1: open the URL.
	var openStderr bytes.Buffer
	openCmd := exec.Command("agent-browser", append(sessionFlags, "open", conn.URL)...)
	openCmd.Stderr = &openStderr
	if err := openCmd.Run(); err != nil {
		return nil, fmt.Errorf("browser fetcher open: %s (stderr: %s)", err, openStderr.String())
	}

	// Step 2: wait for page load — best-effort; fall back to a plain sleep if
	// this agent-browser version doesn't support the wait sub-command.
	waitCmd := exec.Command("agent-browser", append(sessionFlags, "wait", "2000")...)
	if err := waitCmd.Run(); err != nil {
		time.Sleep(2 * time.Second)
	}

	// Step 3: extract data — eval for JS prompts, snapshot for natural language.
	// Capture stdout only from this final step.
	var stdout, finalStderr bytes.Buffer
	var finalArgs []string
	if isJavaScript(conn.Prompt) {
		// Use eval for JS-like prompts, snapshot for natural language prompts
		finalArgs = append(sessionFlags, "eval", conn.Prompt)
	} else {
		// For natural language prompts, take a snapshot — the sync engine
		// will pass this to the browser-operator agent for extraction
		finalArgs = append(sessionFlags, "snapshot")
	}
	finalCmd := exec.Command("agent-browser", finalArgs...)
	finalCmd.Stdout = &stdout
	finalCmd.Stderr = &finalStderr
	if err := finalCmd.Run(); err != nil {
		return nil, fmt.Errorf("browser fetcher extract: %s (stderr: %s)", err, finalStderr.String())
	}

	output := bytes.TrimSpace(stdout.Bytes())

	// Validate/wrap as JSON
	if !json.Valid(output) {
		wrapped, _ := json.Marshal(string(output))
		output = wrapped
	}

	return &FetchResult{
		Data:      json.RawMessage(output),
		FetchedAt: time.Now(),
	}, nil
}

// isJavaScript is a heuristic to detect if a prompt is JS code vs natural language.
func isJavaScript(prompt string) bool {
	p := strings.TrimSpace(prompt)
	return strings.HasPrefix(p, "JSON.stringify") ||
		strings.HasPrefix(p, "document.") ||
		strings.HasPrefix(p, "(function") ||
		strings.HasPrefix(p, "(() =>")
}