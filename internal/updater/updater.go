package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	update "github.com/inconshreveable/go-update"
)

const githubRepo = "kenotron-ms/agent-daemon"

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

// LatestRelease fetches the latest release tag and the download URL for the
// current platform's binary from GitHub Releases.
func LatestRelease() (version string, downloadURL string, err error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	client := &http.Client{Timeout: 10 * time.Second}

	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("fetch release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", "", fmt.Errorf("parse release: %w", err)
	}

	version = strings.TrimPrefix(rel.TagName, "v")

	// Find the right asset for the current OS/arch.
	// Release assets are named: agent-daemon-<os>-<arch>[.exe]
	wantSuffix := fmt.Sprintf("agent-daemon-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		wantSuffix += ".exe"
	}

	for _, a := range rel.Assets {
		if strings.EqualFold(a.Name, wantSuffix) {
			return version, a.BrowserDownloadURL, nil
		}
	}

	return version, "", fmt.Errorf("no binary found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, rel.TagName)
}

// Apply downloads the binary at downloadURL and atomically replaces the
// currently running executable.
func Apply(downloadURL string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	return update.Apply(resp.Body, update.Options{})
}

// IsNewer returns true if candidate is a higher semver than current.
// Both should be plain "X.Y.Z" strings (no "v" prefix).
func IsNewer(current, candidate string) bool {
	cv := parseVer(current)
	nv := parseVer(candidate)
	for i := range cv {
		if nv[i] > cv[i] {
			return true
		}
		if nv[i] < cv[i] {
			return false
		}
	}
	return false
}

func parseVer(v string) [3]int {
	var major, minor, patch int
	fmt.Sscanf(v, "%d.%d.%d", &major, &minor, &patch)
	return [3]int{major, minor, patch}
}
