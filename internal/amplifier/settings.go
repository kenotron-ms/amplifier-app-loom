package amplifier

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// settingsPath returns the path to ~/.amplifier/settings.yaml.
func settingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".amplifier", "settings.yaml"), nil
}

// amplifierSettings mirrors ~/.amplifier/settings.yaml (global) or
// <project>/.amplifier/settings.yaml (project-level).
type amplifierSettings struct {
	Bundle struct {
		Active string            `yaml:"active"` // primary active bundle name
		Added  map[string]string `yaml:"added"`  // name → URI (all installed bundles)
		App    []string          `yaml:"app"`    // always-on overlay URIs
	} `yaml:"bundle"`
}

// GlobalBundleState is the full bundle state read from ~/.amplifier/settings.yaml.
type GlobalBundleState struct {
	Active string            // name of the primary active bundle
	Added  map[string]string // name → URI for every installed bundle
	App    []string          // URI list of always-on app overlays
}

// ReadGlobalBundleSettings reads all three bundle state fields from the global
// settings file. Missing file returns zero-value state (not an error).
func ReadGlobalBundleSettings() (GlobalBundleState, error) {
	path, err := settingsPath()
	if err != nil {
		return GlobalBundleState{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return GlobalBundleState{}, nil
		}
		return GlobalBundleState{}, err
	}
	var s amplifierSettings
	if err := yaml.Unmarshal(data, &s); err != nil {
		return GlobalBundleState{}, err
	}
	return GlobalBundleState{
		Active: s.Bundle.Active,
		Added:  s.Bundle.Added,
		App:    s.Bundle.App,
	}, nil
}

// ReadAppBundles returns the list of app bundle specs currently in
// ~/.amplifier/settings.yaml under bundle.app.
func ReadAppBundles() ([]string, error) {
	state, err := ReadGlobalBundleSettings()
	if err != nil {
		return nil, err
	}
	return state.App, nil
}

// SetGlobalActive sets or clears bundle.active in ~/.amplifier/settings.yaml.
// Pass "" to remove the active key (Amplifier falls back to foundation).
// Uses line-level patching to preserve all other settings and comments.
func SetGlobalActive(name string) error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && name == "" {
			return nil
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	inBundle := false
	foundActive := false

	for i, line := range lines {
		stripped := strings.TrimSpace(line)
		if stripped == "bundle:" {
			inBundle = true
			continue
		}
		// Leave bundle section when we hit a top-level (non-indented) key
		if inBundle && len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			inBundle = false
		}
		if inBundle && strings.HasPrefix(stripped, "active:") {
			foundActive = true
			if name == "" {
				lines[i] = "" // blank the line; cleaned up below
			} else {
				indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				lines[i] = indent + "active: " + name
			}
			break
		}
	}

	// If setting a name and no active: line existed, insert one after "bundle:"
	if !foundActive && name != "" {
		for i, line := range lines {
			if strings.TrimSpace(line) == "bundle:" {
				rest := append([]string{"  active: " + name}, lines[i+1:]...)
				lines = append(lines[:i+1], rest...)
				break
			}
		}
	}

	result := strings.Join(lines, "\n")
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return os.WriteFile(path, []byte(result), 0o644)
}

// IsAppBundle reports whether spec is currently in bundle.app.
func IsAppBundle(spec string) (bool, error) {
	specs, err := ReadAppBundles()
	if err != nil {
		return false, err
	}
	spec = strings.TrimSpace(spec)
	for _, s := range specs {
		if strings.TrimSpace(s) == spec {
			return true, nil
		}
	}
	return false, nil
}
