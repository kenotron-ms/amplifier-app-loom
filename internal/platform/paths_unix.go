//go:build !windows

package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

func DataDir() string {
	if runtime.GOOS == "darwin" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "loom")
	}
	// Linux: XDG_DATA_HOME or ~/.local/share
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "loom")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "loom")
}

func DBPath() string {
	return filepath.Join(DataDir(), "loom.db")
}

func ConfigPath() string {
	return filepath.Join(DataDir(), "config.json")
}

func LogPath() string {
	return filepath.Join(DataDir(), "loom.log")
}

func PIDPath() string {
	return filepath.Join(DataDir(), "loom.pid")
}
