package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	kservice "github.com/kardianos/service"

	internalsvc "github.com/ms/amplifier-app-loom/internal/service"
)

const githubRepo = "kenotron-ms/amplifier-app-loom"

// State represents the current update lifecycle state.
type State string

const (
	// StateIdle is the initial state — no check has been attempted yet.
	StateIdle State = "idle"
	// StateChecking means a GitHub API request is in flight.
	StateChecking State = "checking"
	// StateDownloading means the binary is being downloaded to <exe>.tmp.
	StateDownloading State = "downloading"
	// StateReady means the binary has been staged and verified; ready to apply.
	StateReady State = "ready"
	// StateApplying means the binary swap + service reinstall is in progress.
	StateApplying State = "applying"
	// StateFailed means the last check or download failed.
	StateFailed State = "failed"
	// StateUpToDate means the running version is already the latest.
	StateUpToDate State = "up-to-date"
)

// Updater manages the squirrel-style auto-update lifecycle:
//
//	check → download → stage → (user triggers) → apply
//
// The tray or CLI owns the Updater and calls CheckAndStage in the background.
// When state reaches StateReady, the user is notified; Apply() then does the
// full uninstall / binary-swap / reinstall / re-exec dance.
type Updater struct {
	mu          sync.RWMutex
	state       State
	latestVer   string
	currentVer  string
	stagingPath string // absolute path to the downloaded .tmp binary
	err         error
	onChange    func(s State, ver string)
}

// New creates a new Updater. onChange is called on every state transition
// and may be called from any goroutine.
func New(currentVer string, onChange func(State, string)) *Updater {
	return &Updater{
		currentVer: currentVer,
		state:      StateIdle,
		onChange:   onChange,
	}
}

// State returns the current update state (safe for concurrent use).
func (u *Updater) State() State {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.state
}

// LatestVersion returns the latest release version found during the last check.
func (u *Updater) LatestVersion() string {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.latestVer
}

// Err returns the last error (meaningful when State == StateFailed).
func (u *Updater) Err() error {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.err
}

func (u *Updater) setState(s State, ver string) {
	u.mu.Lock()
	u.state = s
	if ver != "" {
		u.latestVer = ver
	}
	u.mu.Unlock()
	if u.onChange != nil {
		u.onChange(s, ver)
	}
}

// CheckAndStage checks GitHub for a newer release. If one is found, it
// downloads the binary to <exe>.tmp and verifies its SHA-256 checksum.
//
// State transitions:
//
//	idle/failed/up-to-date → checking → up-to-date
//	idle/failed/up-to-date → checking → downloading → ready
//	                                  → downloading → failed
func (u *Updater) CheckAndStage(ctx context.Context) error {
	// Don't interrupt an in-flight download or apply.
	u.mu.RLock()
	cur := u.state
	u.mu.RUnlock()
	if cur == StateApplying || cur == StateDownloading || cur == StateChecking || cur == StateReady {
		return nil
	}

	u.setState(StateChecking, "")

	latest, downloadURL, checksumURL, err := latestRelease(ctx)
	if err != nil {
		u.mu.Lock()
		u.state = StateFailed
		u.err = err
		u.mu.Unlock()
		if u.onChange != nil {
			u.onChange(StateFailed, "")
		}
		return fmt.Errorf("check for updates: %w", err)
	}

	if !IsNewer(u.currentVer, latest) {
		u.setState(StateUpToDate, latest)
		return nil
	}

	// Newer version available — download and stage.
	u.setState(StateDownloading, latest)

	exePath, err := os.Executable()
	if err != nil {
		u.mu.Lock()
		u.state = StateFailed
		u.err = err
		u.mu.Unlock()
		if u.onChange != nil {
			u.onChange(StateFailed, latest)
		}
		return fmt.Errorf("resolve executable path: %w", err)
	}
	stagingPath := exePath + ".tmp"

	if err := downloadAndVerify(ctx, downloadURL, checksumURL, stagingPath); err != nil {
		_ = os.Remove(stagingPath) // clean up partial download
		u.mu.Lock()
		u.state = StateFailed
		u.err = err
		u.mu.Unlock()
		if u.onChange != nil {
			u.onChange(StateFailed, latest)
		}
		return fmt.Errorf("download update: %w", err)
	}

	u.mu.Lock()
	u.stagingPath = stagingPath
	u.state = StateReady
	u.mu.Unlock()
	if u.onChange != nil {
		u.onChange(StateReady, latest)
	}

	slog.Info("updater: update staged and ready", "version", latest, "staging", stagingPath)
	return nil
}

// Apply performs the full squirrel-style update:
//  1. Detect current service install level
//  2. Stop and uninstall the service
//  3. Atomically swap the staged binary into place
//  4. Reinstall and start the service (with the new binary)
//  5. Remove the .old backup
//
// It returns the path of the new binary so the caller can re-exec after doing
// its own cleanup (e.g. quitting the systray loop before re-execing).
// Use ReExec to perform the actual process replacement.
func (u *Updater) Apply() (newExePath string, err error) {
	u.mu.Lock()
	if u.state != StateReady {
		s := u.state
		u.mu.Unlock()
		return "", fmt.Errorf("update not staged (current state: %s)", s)
	}
	stagingPath := u.stagingPath
	u.state = StateApplying
	u.mu.Unlock()
	if u.onChange != nil {
		u.onChange(StateApplying, u.latestVer)
	}

	// Capture the executable path before any renames.
	// On Linux, os.Executable() reads /proc/self/exe which tracks the inode;
	// after renaming the inode moves to .old. We resolve it now so we have
	// the canonical path that the new binary will live at after the swap.
	exePath, err := os.Executable()
	if err != nil {
		return "", u.fail(fmt.Errorf("resolve executable: %w", err))
	}

	// ── 1. Detect install level ───────────────────────────────────────────────
	level, err := internalsvc.DetectInstallLevel()
	if err != nil {
		slog.Warn("updater: could not detect install level, defaulting to user", "err", err)
		level = internalsvc.LevelUser
	}

	// ── 2. Stop + uninstall service ───────────────────────────────────────────
	// Errors here are non-fatal: the service may simply not be installed.
	if svc, err := internalsvc.NewServiceForControl(level); err == nil {
		if err := kservice.Control(svc, "stop"); err != nil {
			slog.Info("updater: stop service (may already be stopped)", "err", err)
		}
		if err := kservice.Control(svc, "uninstall"); err != nil {
			slog.Info("updater: uninstall service (may not be installed)", "err", err)
		}
	}

	// ── 3. Atomic binary swap ─────────────────────────────────────────────────
	oldPath := exePath + ".old"

	// Remove any leftover .old from a previous run before we attempt the rename.
	_ = os.Remove(oldPath)

	if err := os.Rename(exePath, oldPath); err != nil {
		return "", u.fail(fmt.Errorf("rename current binary to .old: %w", err))
	}
	if err := os.Rename(stagingPath, exePath); err != nil {
		// Attempt rollback so the installation isn't left broken.
		_ = os.Rename(oldPath, exePath)
		return "", u.fail(fmt.Errorf("rename .tmp to binary: %w", err))
	}
	if err := os.Chmod(exePath, 0755); err != nil {
		slog.Warn("updater: chmod new binary", "err", err)
	}

	slog.Info("updater: binary swapped", "path", exePath, "version", u.latestVer)

	// ── 4. Reinstall + start service with the new binary ─────────────────────
	// Use the explicit exePath so we don't rely on os.Executable() returning
	// the right value after the rename (behaviour differs by OS).
	if svc, err := internalsvc.NewServiceForControlWithExe(level, exePath); err == nil {
		if err := kservice.Control(svc, "install"); err != nil {
			slog.Warn("updater: reinstall service", "err", err)
		} else {
			if err := kservice.Control(svc, "start"); err != nil {
				slog.Warn("updater: start service after reinstall", "err", err)
			}
		}
	}

	// ── 5. Clean up .old ──────────────────────────────────────────────────────
	_ = os.Remove(oldPath)

	// Return the new binary path. The caller is responsible for re-execing
	// after doing its own cleanup (e.g. quitting the systray loop).
	return exePath, nil
}

func (u *Updater) fail(err error) error {
	u.mu.Lock()
	u.state = StateFailed
	u.err = err
	ver := u.latestVer
	u.mu.Unlock()
	if u.onChange != nil {
		u.onChange(StateFailed, ver)
	}
	slog.Error("updater: apply failed", "err", err)
	return err
}

// ReExec replaces the current process with exePath running subcommand.
// Call this after Apply() once any cleanup (e.g. systray.Quit()) is done.
// On Unix, uses syscall.Exec — never returns on success.
// On Windows, spawns a new process and exits.
func ReExec(exePath, subcommand string) {
	reExec(exePath, subcommand)
}

// CleanupOldBinary removes any <exe>.old file left by a previous update.
// Call once at startup (before any update check) to keep the directory clean.
func CleanupOldBinary() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	oldPath := exePath + ".old"
	if _, err := os.Stat(oldPath); err == nil {
		slog.Info("updater: removing leftover .old binary from previous update", "path", oldPath)
		_ = os.Remove(oldPath)
	}
}

// reExec replaces the current process with exePath running subcommand.
// On Unix, uses syscall.Exec (same PID — never returns on success).
// On Windows, spawns a new process then exits.
func reExec(exePath, subcommand string) {
	args := []string{exePath}
	if subcommand != "" {
		args = append(args, subcommand)
	}
	slog.Info("updater: re-exec into new binary", "path", exePath, "subcommand", subcommand)

	if runtime.GOOS == "windows" {
		cmd := exec.Command(exePath, subcommand)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			slog.Error("updater: re-exec (windows) failed", "err", err)
			return
		}
		os.Exit(0)
	}

	// Unix: in-place exec — replaces process image, preserves PID.
	if err := syscall.Exec(exePath, args, os.Environ()); err != nil {
		slog.Error("updater: re-exec failed", "err", err)
	}
}

// ─────────────────────────── GitHub API ──────────────────────────────────────

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

// LatestRelease fetches the latest release tag and download URL for the
// current platform from GitHub Releases. It is the simple public API used
// by code that only needs to check for a newer version without staging.
func LatestRelease() (version string, downloadURL string, err error) {
	v, u, _, e := latestRelease(context.Background())
	return v, u, e
}

func latestRelease(ctx context.Context) (version, downloadURL, checksumURL string, err error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	client := &http.Client{Timeout: 10 * time.Second}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("fetch release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", "", "", fmt.Errorf("parse release: %w", err)
	}

	version = strings.TrimPrefix(rel.TagName, "v")

	wantBinary := fmt.Sprintf("loom-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		wantBinary += ".exe"
	}

	for _, a := range rel.Assets {
		switch {
		case strings.EqualFold(a.Name, wantBinary):
			downloadURL = a.BrowserDownloadURL
		case a.Name == "checksums.txt":
			checksumURL = a.BrowserDownloadURL
		}
	}

	if downloadURL == "" {
		return version, "", "", fmt.Errorf(
			"no binary found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, rel.TagName)
	}

	return version, downloadURL, checksumURL, nil
}

// ─────────────────────────── Download + verify ───────────────────────────────

func downloadAndVerify(ctx context.Context, downloadURL, checksumURL, destPath string) error {
	client := &http.Client{Timeout: 5 * time.Minute}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("create staging file: %w", err)
	}

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("write staging file: %w", err)
	}
	f.Close()

	got := hex.EncodeToString(h.Sum(nil))

	// Verify checksum when checksums.txt is available. Non-fatal if unavailable.
	if checksumURL != "" {
		expected, err := fetchExpectedChecksum(ctx, checksumURL)
		if err != nil {
			slog.Warn("updater: could not fetch checksums.txt, skipping verification", "err", err)
		} else if expected != "" {
			if !strings.EqualFold(got, expected) {
				return fmt.Errorf("checksum mismatch: got %s, want %s", got, expected)
			}
			slog.Info("updater: checksum verified", "sha256", got)
		}
	}

	return nil
}

func fetchExpectedChecksum(ctx context.Context, checksumURL string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Format: "<sha256>  loom-<os>-<arch>[.exe]"
	wantFilename := fmt.Sprintf("loom-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		wantFilename += ".exe"
	}

	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && strings.EqualFold(parts[1], wantFilename) {
			return parts[0], nil
		}
	}

	return "", nil // no matching entry — verification skipped
}

// ─────────────────────────── Semver ──────────────────────────────────────────

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
