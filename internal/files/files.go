package files

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Entry is a single directory listing item.
type Entry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

// Browser provides path-scoped read-only file access.
type Browser struct {
	root string
}

// New creates a Browser rooted at root (must be absolute).
// The root is canonicalised via filepath.EvalSymlinks so that symlink
// checks inside resolve() compare against the real path on disk.
func New(root string) *Browser {
	clean := filepath.Clean(root)
	if real, err := filepath.EvalSymlinks(clean); err == nil {
		clean = real
	}
	return &Browser{root: clean}
}

// resolve validates rel and returns its absolute path, rejecting traversal.
func (b *Browser) resolve(rel string) (string, error) {
	abs := filepath.Clean(filepath.Join(b.root, rel))
	// Reject traversal before hitting the filesystem.
	if abs != b.root && !strings.HasPrefix(abs, b.root+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside root", rel)
	}
	// Resolve symlinks and re-check so a symlink can't escape the root.
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", rel, err)
	}
	if real != b.root && !strings.HasPrefix(real, b.root+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes root via symlink", rel)
	}
	return real, nil
}

// List returns directory entries at rel (empty string = root).
func (b *Browser) List(rel string) ([]Entry, error) {
	abs, err := b.resolve(rel)
	if err != nil {
		return nil, err
	}
	des, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	result := make([]Entry, 0, len(des))
	for _, de := range des {
		info, _ := de.Info()
		var size int64
		if info != nil && !de.IsDir() {
			size = info.Size()
		}
		result = append(result, Entry{Name: de.Name(), IsDir: de.IsDir(), Size: size})
	}
	return result, nil
}

// Read returns the contents of the file at rel.
func (b *Browser) Read(rel string) ([]byte, error) {
	abs, err := b.resolve(rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(abs)
}
