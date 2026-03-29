//go:build windows

package platform

import (
	"os"
	"path/filepath"
)

func DataDir() string {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		home, _ := os.UserHomeDir()
		appdata = filepath.Join(home, "AppData", "Roaming")
	}
	return filepath.Join(appdata, "loom")
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
