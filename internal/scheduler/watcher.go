package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/ms/amplifier-app-loom/internal/types"
)

// startWatcher launches a file watcher for a watch-triggered job and calls dispatch
// whenever a matching event fires (after debounce). It blocks until ctx is cancelled.
func (s *Scheduler) startWatcher(ctx context.Context, job *types.Job) {
	cfg := job.Watch
	if cfg == nil || cfg.Path == "" {
		slog.Error("watch job has no path configured", "job", job.Name)
		return
	}

	debounce := 300 * time.Millisecond
	if cfg.Debounce != "" {
		if d, err := time.ParseDuration(cfg.Debounce); err == nil {
			debounce = d
		}
	}

	filter := buildEventFilter(cfg.Events)

	if cfg.Mode == "poll" {
		s.runPollWatcher(ctx, job, cfg, filter, debounce)
	} else {
		s.runNotifyWatcher(ctx, job, cfg, filter, debounce)
	}
}

// ── Notify watcher (OS-level: inotify / FSEvents / kqueue) ───────────────────

func (s *Scheduler) runNotifyWatcher(ctx context.Context, job *types.Job, cfg *types.WatchConfig, filter eventFilter, debounce time.Duration) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("failed to create fsnotify watcher", "job", job.Name, "err", err)
		return
	}
	defer w.Close()

	if err := addWatchPaths(w, cfg.Path, cfg.Recursive); err != nil {
		slog.Error("failed to watch path", "job", job.Name, "path", cfg.Path, "err", err)
		return
	}

	slog.Info("notify watcher started", "job", job.Name, "path", cfg.Path, "recursive", cfg.Recursive)

	// lastSnapshot is used to verify real content changes before dispatching,
	// preventing spurious fires from editors that touch mtime on open (e.g. VSCode).
	lastSnapshot := snapshotDir(cfg.Path, cfg.Recursive)

	var (
		mu    sync.Mutex
		timer *time.Timer
	)

	fire := func() {
		current := snapshotDir(cfg.Path, cfg.Recursive)
		mu.Lock()
		prev := lastSnapshot
		lastSnapshot = current
		mu.Unlock()

		changed, changedPaths := diffSnapshot(prev, current, filter)
		if !changed {
			slog.Debug("notify watcher: no real content change, skipping dispatch", "job", job.Name)
			return
		}

		j, err := s.store.GetJob(ctx, job.ID)
		if err != nil || !j.Enabled {
			return
		}
		j.RuntimeEnv = map[string]string{"JOB_WATCH_PATH": cfg.Path}
		if len(changedPaths) > 0 {
			j.RuntimeEnv["JOB_EVENT_PATH"] = changedPaths[0]
		}
		s.dispatch(j)
	}

	scheduleDispatch := func() {
		mu.Lock()
		defer mu.Unlock()
		if timer != nil {
			timer.Reset(debounce)
		} else {
			timer = time.AfterFunc(debounce, fire)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.Events:
			if !ok {
				return
			}
			if !filter(event.Op) {
				continue
			}
			slog.Debug("fs event", "job", job.Name, "op", event.Op, "name", event.Name)

			// If a new directory was created and we're recursive, watch it
			if cfg.Recursive && event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = w.Add(event.Name)
				}
			}
			scheduleDispatch()

		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			slog.Warn("watcher error", "job", job.Name, "err", err)
		}
	}
}

// ── Poll watcher ──────────────────────────────────────────────────────────────

type fileState struct {
	hash    string
	modTime time.Time
	size    int64
}

func (s *Scheduler) runPollWatcher(ctx context.Context, job *types.Job, cfg *types.WatchConfig, filter eventFilter, debounce time.Duration) {
	interval := 2 * time.Second
	if cfg.PollInterval != "" {
		if d, err := time.ParseDuration(cfg.PollInterval); err == nil && d > 0 {
			interval = d
		}
	}

	slog.Info("poll watcher started", "job", job.Name, "path", cfg.Path, "interval", interval)

	// Build initial snapshot
	snapshot := snapshotDir(cfg.Path, cfg.Recursive)

	var (
		mu           sync.Mutex
		pendingPaths []string
		timer        *time.Timer
	)

	fire := func() {
		mu.Lock()
		paths := pendingPaths
		pendingPaths = nil
		mu.Unlock()
		j, err := s.store.GetJob(ctx, job.ID)
		if err != nil || !j.Enabled {
			return
		}
		j.RuntimeEnv = map[string]string{"JOB_WATCH_PATH": cfg.Path}
		if len(paths) > 0 {
			j.RuntimeEnv["JOB_EVENT_PATH"] = paths[0]
		}
		s.dispatch(j)
	}

	scheduleDispatch := func(paths []string) {
		mu.Lock()
		defer mu.Unlock()
		pendingPaths = append(pendingPaths, paths...)
		if timer != nil {
			timer.Reset(debounce)
		} else {
			timer = time.AfterFunc(debounce, fire)
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := snapshotDir(cfg.Path, cfg.Recursive)
			if changed, paths := diffSnapshot(snapshot, current, filter); changed {
				snapshot = current
				scheduleDispatch(paths)
			}
		}
	}
}

func snapshotDir(root string, recursive bool) map[string]fileState {
	snap := make(map[string]fileState)
	info, err := os.Stat(root)
	if err != nil {
		return snap
	}
	if !info.IsDir() {
		snap[root] = fileStateFor(root, info)
		return snap
	}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && !recursive {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		snap[path] = fileStateFor(path, info)
		return nil
	})
	return snap
}

func fileStateFor(path string, info os.FileInfo) fileState {
	return fileState{
		hash:    quickHash(path, info),
		modTime: info.ModTime(),
		size:    info.Size(),
	}
}

// quickHash returns a content-based hash for the file so that mtime-only changes
// (e.g. editors touching a file on open) do not count as modifications.
// For files ≤ 4 MB: full SHA256. For larger files: hash of first+last 256 KB + size,
// which catches truncation and most edits without reading the whole file.
func quickHash(path string, info os.FileInfo) string {
	size := info.Size()
	f, err := os.Open(path)
	if err != nil {
		// Fallback: size only (at least mtime changes won't matter)
		return fmt.Sprintf("size:%d", size)
	}
	defer f.Close()

	h := sha256.New()
	const fullThreshold = 4 * 1024 * 1024   // 4 MB
	const chunkSize = 256 * 1024             // 256 KB
	if size <= fullThreshold {
		if _, err := io.Copy(h, f); err == nil {
			return hex.EncodeToString(h.Sum(nil))
		}
	} else {
		// Read first chunk
		if _, err := io.CopyN(h, f, chunkSize); err != nil {
			return fmt.Sprintf("size:%d", size)
		}
		// Read last chunk
		if _, err := f.Seek(-chunkSize, io.SeekEnd); err == nil {
			_, _ = io.CopyN(h, f, chunkSize)
		}
		// Include size so truncation is detected
		fmt.Fprintf(h, "|size:%d", size)
		return hex.EncodeToString(h.Sum(nil))
	}
	return fmt.Sprintf("size:%d", size)
}

// diffSnapshot returns true if current differs from previous.
// The filter decides which types of changes count (create/write/remove).
func diffSnapshot(prev, curr map[string]fileState, filter eventFilter) (bool, []string) {
	var changed []string
	for path, cs := range curr {
		if ps, ok := prev[path]; !ok {
			// new file
			if filter(fsnotify.Create) {
				changed = append(changed, path)
			}
		} else if cs.hash != ps.hash {
			// modified
			if filter(fsnotify.Write) {
				changed = append(changed, path)
			}
		}
	}
	for path := range prev {
		if _, ok := curr[path]; !ok {
			// deleted
			if filter(fsnotify.Remove) {
				changed = append(changed, path)
			}
		}
	}
	return len(changed) > 0, changed
}

// ── Helpers ───────────────────────────────────────────────────────────────────

type eventFilter func(fsnotify.Op) bool

func buildEventFilter(events []string) eventFilter {
	if len(events) == 0 {
		return func(fsnotify.Op) bool { return true }
	}
	ops := fsnotify.Op(0)
	for _, e := range events {
		switch strings.ToLower(e) {
		case "create":
			ops |= fsnotify.Create
		case "write", "modify":
			ops |= fsnotify.Write
		case "remove", "delete":
			ops |= fsnotify.Remove
		case "rename":
			ops |= fsnotify.Rename
		case "chmod":
			ops |= fsnotify.Chmod
		}
	}
	return func(op fsnotify.Op) bool { return op&ops != 0 }
}

func addWatchPaths(w *fsnotify.Watcher, root string, recursive bool) error {
	if err := w.Add(root); err != nil {
		return err
	}
	if !recursive {
		return nil
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == root {
			return nil
		}
		return w.Add(path)
	})
}
