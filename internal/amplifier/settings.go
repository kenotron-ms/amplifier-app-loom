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

// amplifierSettings mirrors the subset of ~/.amplifier/settings.yaml we care about.
type amplifierSettings struct {
	Bundle struct {
		App []string `yaml:"app"`
	} `yaml:"bundle"`
}

// ReadAppBundles returns the list of app bundle specs currently in
// ~/.amplifier/settings.yaml under bundle.app.
func ReadAppBundles() ([]string, error) {
	path, err := settingsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var s amplifierSettings
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return s.Bundle.App, nil
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
