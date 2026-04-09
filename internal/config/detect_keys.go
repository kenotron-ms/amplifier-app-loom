package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// DetectedKeys holds API keys discovered from the local environment, along
// with the source of each key for diagnostic logging.
type DetectedKeys struct {
	AnthropicKey    string
	OpenAIKey       string
	AnthropicSource string // e.g. "env", "~/.anthropic/api_key", "~/.zshrc"
	OpenAISource    string
}

// HasAny returns true if at least one key was found.
func (d DetectedKeys) HasAny() bool {
	return d.AnthropicKey != "" || d.OpenAIKey != ""
}

// DetectAPIKeys scans the local environment for AI provider API keys without
// requiring the caller to have any particular shell environment.  This is
// necessary because the daemon runs as a LaunchAgent/systemd service and
// inherits a minimal env that omits user shell exports.
//
// Sources are checked in priority order; the first value found for each key
// wins:
//
//  1. Process env  — ANTHROPIC_API_KEY / OPENAI_API_KEY (set when invoked
//     from a shell, e.g. during `loom install` or `loom config absorb-env`)
//  2. ~/.amplifier/keys.env — Amplifier's own key file (KEY=value format)
//  3. ~/.anthropic/api_key  — Anthropic CLI single-line key file
//  4. ~/.env                — common dotenv convention (KEY=value format)
//  5. Shell dotfiles        — ~/.zshrc, ~/.zshenv, ~/.zprofile,
//     ~/.bash_profile, ~/.bashrc, ~/.profile  (export KEY=value)
func DetectAPIKeys() DetectedKeys {
	var d DetectedKeys

	// ── 1. Process environment ────────────────────────────────────────────
	if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" {
		d.AnthropicKey = k
		d.AnthropicSource = "env"
	}
	if k := os.Getenv("OPENAI_API_KEY"); k != "" {
		d.OpenAIKey = k
		d.OpenAISource = "env"
	}
	if d.complete() {
		return d
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return d
	}

	// ── 2. ~/.amplifier/keys.env ─────────────────────────────────────────
	d.mergeEnvFile(filepath.Join(home, ".amplifier", "keys.env"), "~/.amplifier/keys.env")
	if d.complete() {
		return d
	}

	// ── 3. ~/.anthropic/api_key  (Anthropic CLI format: single raw line) ─
	if d.AnthropicKey == "" {
		if key := readSingleLineFile(filepath.Join(home, ".anthropic", "api_key")); key != "" {
			d.AnthropicKey = key
			d.AnthropicSource = "~/.anthropic/api_key"
		}
	}
	if d.complete() {
		return d
	}

	// ── 4. ~/.env ────────────────────────────────────────────────────────
	d.mergeEnvFile(filepath.Join(home, ".env"), "~/.env")
	if d.complete() {
		return d
	}

	// ── 5. Shell dotfiles ─────────────────────────────────────────────────
	for _, name := range []string{".zshrc", ".zshenv", ".zprofile", ".bash_profile", ".bashrc", ".profile"} {
		d.mergeShellFile(filepath.Join(home, name), "~/"+name)
		if d.complete() {
			break
		}
	}

	return d
}

// complete returns true once both keys are found.
func (d *DetectedKeys) complete() bool {
	return d.AnthropicKey != "" && d.OpenAIKey != ""
}

// mergeEnvFile parses a KEY=value file and fills any missing keys.
func (d *DetectedKeys) mergeEnvFile(path, label string) {
	kv := parseEnvFile(path)
	if d.AnthropicKey == "" {
		if v := kv["ANTHROPIC_API_KEY"]; v != "" {
			d.AnthropicKey = v
			d.AnthropicSource = label
		}
	}
	if d.OpenAIKey == "" {
		if v := kv["OPENAI_API_KEY"]; v != "" {
			d.OpenAIKey = v
			d.OpenAISource = label
		}
	}
}

// mergeShellFile parses a shell config file and fills any missing keys.
func (d *DetectedKeys) mergeShellFile(path, label string) {
	kv := parseShellExports(path)
	if d.AnthropicKey == "" {
		if v := kv["ANTHROPIC_API_KEY"]; v != "" {
			d.AnthropicKey = v
			d.AnthropicSource = label
		}
	}
	if d.OpenAIKey == "" {
		if v := kv["OPENAI_API_KEY"]; v != "" {
			d.OpenAIKey = v
			d.OpenAISource = label
		}
	}
}

// readSingleLineFile reads the first non-empty line from a file, trimming
// surrounding whitespace.  Used for ~/.anthropic/api_key.
func readSingleLineFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// parseEnvFile parses KEY=value (and export KEY=value) lines from a dotenv-
// style file, stripping surrounding quotes and ignoring blank lines / comments.
func parseEnvFile(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	result := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip optional leading "export "
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = stripInlineComment(strings.TrimSpace(val))
		val = strings.Trim(val, `"'`)
		if key != "" && val != "" {
			result[key] = val
		}
	}
	return result
}

// parseShellExports extracts export KEY=value assignments from a shell config
// file using line-by-line scanning.  Handles all common forms:
//
//	export KEY=value
//	export KEY="value"
//	export KEY='value'
//
// Only handles simple single-line exports (no multi-line, subshell, or
// parameter-expansion values).
func parseShellExports(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	result := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Require the "export " prefix; strip it then fall through to KEY=val parsing.
		if !strings.HasPrefix(line, "export ") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export"))
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = stripInlineComment(strings.TrimSpace(val))
		val = strings.Trim(val, `"'`)
		if key != "" && val != "" {
			result[key] = val
		}
	}
	return result
}

// stripInlineComment removes a trailing  # comment  from a value string.
// Only strips if the # is preceded by whitespace (avoids breaking keys that
// contain # characters, though API keys currently don't).
func stripInlineComment(s string) string {
	if idx := strings.Index(s, " #"); idx != -1 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}
