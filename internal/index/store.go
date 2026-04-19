package index

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// DefaultDir returns the default directory for the bundle index.
func DefaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".amplifier", "github-bundle-index")
	}
	return filepath.Join(home, ".amplifier", "github-bundle-index")
}

// LoadIndex reads index.json from dir. Returns an empty struct if the file does
// not exist.
func LoadIndex(dir string) (*IndexFile, error) {
	data, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return &IndexFile{Version: 1, Repos: make(map[string]Entry)}, nil
		}
		return nil, err
	}
	var idx IndexFile
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	if idx.Repos == nil {
		idx.Repos = make(map[string]Entry)
	}
	return &idx, nil
}

// LoadState reads state.json from dir. Returns an empty struct if the file does
// not exist.
func LoadState(dir string) (*StateFile, error) {
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return &StateFile{Version: 1, Repos: make(map[string]RepoState)}, nil
		}
		return nil, err
	}
	var st StateFile
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	if st.Repos == nil {
		st.Repos = make(map[string]RepoState)
	}
	return &st, nil
}

// SaveIndex marshals idx and writes it to dir/index.json, creating dir if needed.
func SaveIndex(dir string, idx *IndexFile) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "index.json"), data, 0644)
}

// SaveState marshals st and writes it to dir/state.json, creating dir if needed.
func SaveState(dir string, st *StateFile) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "state.json"), data, 0644)
}

// ── Sources config ─────────────────────────────────────────────────────────────

const sourcesFile = "sources.json"

// LoadSources reads sources.json from dir, returning an empty Sources if absent.
func LoadSources(dir string) (*Sources, error) {
	path := filepath.Join(dir, sourcesFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Sources{}, nil
	}
	if err != nil {
		return nil, err
	}
	var s Sources
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// SaveSources writes sources.json to dir.
func SaveSources(dir string, s *Sources) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, sourcesFile), append(data, '\n'), 0o644)
}
