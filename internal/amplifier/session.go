// Package amplifier reads metadata from locally-stored Amplifier sessions.
package amplifier

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Meta mirrors the subset of metadata.json we care about.
type Meta struct {
	SessionID       string     `json:"session_id"`
	Name            string     `json:"name"`
	NameGeneratedAt *time.Time `json:"name_generated_at"`
	WorkingDir      string     `json:"working_dir"`
	Created         time.Time  `json:"created"`
}

// projectSlug converts an absolute path to amplifier's project directory slug.
// e.g. "/Users/ken/workspace/ms/loom" → "-Users-ken-workspace-ms-loom"
func projectSlug(projectPath string) string {
	return strings.ReplaceAll(projectPath, "/", "-")
}

// sessionsDir returns the path to amplifier's sessions directory for projectPath.
func sessionsDir(projectPath string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".amplifier", "projects", projectSlug(projectPath), "sessions"), nil
}

// NewestSession returns the metadata of the most-recently-modified session for
// projectPath, optionally filtered to sessions created after `after`.
func NewestSession(projectPath string, after time.Time) (*Meta, error) {
	dir, err := sessionsDir(projectPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	type candidate struct {
		meta    Meta
		modTime time.Time
	}
	var candidates []candidate

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if !after.IsZero() && info.ModTime().Before(after) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name(), "metadata.json"))
		if err != nil {
			continue
		}
		var m Meta
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		candidates = append(candidates, candidate{m, info.ModTime()})
	}

	if len(candidates) == 0 {
		return nil, nil
	}
	// Most recently modified first
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	result := candidates[0].meta
	return &result, nil
}

// WatchName polls for the amplifier session name for projectPath, starting from
// `after`. It calls onChange whenever the name becomes non-empty or changes.
// Stops when ctx is done.
func WatchName(ctx interface{ Done() <-chan struct{} }, projectPath string, after time.Time, onChange func(name string)) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	lastName := ""

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			meta, err := NewestSession(projectPath, after)
			if err != nil || meta == nil || meta.Name == "" {
				continue
			}
			if meta.Name != lastName {
				lastName = meta.Name
				onChange(meta.Name)
			}
		}
	}
}
