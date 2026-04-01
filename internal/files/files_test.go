package files_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ms/amplifier-app-loom/internal/files"
)

func makeTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("makeTree WriteFile: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0755); err != nil {
		t.Fatalf("makeTree MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "util.go"), []byte("package internal"), 0644); err != nil {
		t.Fatalf("makeTree WriteFile: %v", err)
	}
	return root
}

func TestListRoot(t *testing.T) {
	root := makeTree(t)
	b := files.New(root)

	entries, err := b.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestListSubdir(t *testing.T) {
	root := makeTree(t)
	b := files.New(root)

	entries, err := b.List("internal")
	if err != nil {
		t.Fatalf("List internal: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "util.go" {
		t.Fatalf("expected util.go, got %s", entries[0].Name)
	}
}

func TestReadFile(t *testing.T) {
	root := makeTree(t)
	b := files.New(root)

	data, err := b.Read("main.go")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(data) != "package main" {
		t.Fatalf("unexpected content: %s", data)
	}
}

func TestPathTraversalBlocked(t *testing.T) {
	root := makeTree(t)
	b := files.New(root)

	_, err := b.List("../../../etc")
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}

	_, err = b.Read("../../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
}

func TestSymlinkEscapeBlocked(t *testing.T) {
	root := makeTree(t)
	outside := t.TempDir()
	// create a symlink inside root that points outside
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	b := files.New(root)
	_, err := b.List("escape")
	if err == nil {
		t.Fatal("expected error for symlink escape, got nil")
	}
}
