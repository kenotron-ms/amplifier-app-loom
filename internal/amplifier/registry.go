package amplifier

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// BundleRegistryEntry is one entry from ~/.amplifier/registry.json.
// Only the fields loom cares about are decoded; extras are ignored.
type BundleRegistryEntry struct {
	URI       string  `json:"uri"`
	Name      string  `json:"name"`
	Version   *string `json:"version"`
	LoadedAt  *string `json:"loaded_at"`
	LocalPath *string `json:"local_path"`
	IsRoot    bool    `json:"is_root"`
}

// Downloaded reports whether the bundle content is present on disk.
func (e BundleRegistryEntry) Downloaded() bool {
	return e.LocalPath != nil && *e.LocalPath != ""
}

type amplifierRegistry struct {
	Version int                            `json:"version"`
	Bundles map[string]BundleRegistryEntry `json:"bundles"`
}

// ReadBundleRegistry reads ~/.amplifier/registry.json. Returns an empty map
// (not an error) when the file is absent or empty.
func ReadBundleRegistry() (map[string]BundleRegistryEntry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".amplifier", "registry.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]BundleRegistryEntry{}, nil
		}
		return nil, err
	}
	var reg amplifierRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return map[string]BundleRegistryEntry{}, nil // tolerate malformed file
	}
	if reg.Bundles == nil {
		return map[string]BundleRegistryEntry{}, nil
	}
	return reg.Bundles, nil
}
