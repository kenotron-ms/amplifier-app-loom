//go:build darwin && cgo

package tray

// Bundle self-repair: the in-app updater replaces only the binary inside
// Contents/MacOS/loom. Resources like Loom.icns and Info.plist are never
// touched. This file embeds Loom.icns directly in the binary and writes it
// to Contents/Resources/ on every tray startup, ensuring the icon is always
// present even on installs that predate the icon or updated via the updater.

import (
	_ "embed"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

//go:embed resources/Loom.icns
var bundleIcon []byte

// repairBundle checks whether the binary is running inside a .app bundle and,
// if so, ensures Contents/Resources/Loom.icns is present and up to date.
// Safe to call on every launch — it's a no-op if the icon is already current.
func repairBundle() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}

	// Only act when running from inside a .app bundle.
	const marker = ".app/Contents/MacOS/"
	idx := strings.Index(exePath, marker)
	if idx < 0 {
		return
	}

	contentsDir := exePath[:idx+len(".app/Contents/")]
	resourcesDir := filepath.Join(contentsDir, "Resources")
	icnsPath := filepath.Join(resourcesDir, "Loom.icns")

	// Check if the file is already current (same byte count as embedded).
	if info, err := os.Stat(icnsPath); err == nil && info.Size() == int64(len(bundleIcon)) {
		return
	}

	if err := os.MkdirAll(resourcesDir, 0o755); err != nil {
		slog.Warn("bundle repair: cannot create Resources dir", "err", err)
		return
	}

	if err := os.WriteFile(icnsPath, bundleIcon, 0o644); err != nil {
		slog.Warn("bundle repair: cannot write Loom.icns", "err", err)
		return
	}

	slog.Info("bundle repair: wrote Loom.icns", "path", icnsPath)
}
