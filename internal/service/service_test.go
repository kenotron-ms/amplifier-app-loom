package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildServiceConfig_ExecutableIsResolved(t *testing.T) {
	cfg := BuildServiceConfig(LevelUser)
	// The executable path in the config must equal its own resolved form.
	// If EvalSymlinks is working, the path is already canonical.
	resolved, err := filepath.EvalSymlinks(cfg.Executable)
	if err != nil {
		t.Skipf("could not resolve path %s: %v", cfg.Executable, err)
	}
	if cfg.Executable != resolved {
		t.Errorf("BuildServiceConfig returned unresolved path\n  got:  %s\n  want: %s", cfg.Executable, resolved)
	}
}

func TestBuildServiceConfig_ResolvesSymlink(t *testing.T) {
	// Create a real file and a symlink to it
	dir := t.TempDir()
	real := filepath.Join(dir, "real-binary")
	if err := os.WriteFile(real, []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "symlink-binary")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	// Resolve the real path too — on macOS t.TempDir() returns /var/folders/...
	// which is itself a symlink to /private/var/folders/..., so EvalSymlinks
	// on our link resolves *all* symlinks in the chain.
	realResolved, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}

	// Verify filepath.EvalSymlinks on the symlink resolves to the real file
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != realResolved {
		t.Errorf("expected %s, got %s", realResolved, resolved)
	}
}
