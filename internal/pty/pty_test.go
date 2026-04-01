package pty_test

import (
	"testing"
	"time"

	loompty "github.com/ms/amplifier-app-loom/internal/pty"
)

func TestSpawnAndKill(t *testing.T) {
	mgr := loompty.NewManager()

	id, err := mgr.Spawn("test-proc", t.TempDir(), []string{"/bin/sh"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty process ID")
	}

	if !mgr.IsAlive(id) {
		t.Fatal("expected process to be alive after spawn")
	}

	if err := mgr.Kill(id); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	if mgr.IsAlive(id) {
		t.Fatal("expected process to be dead after kill")
	}
}

func TestSpawnDeduplicated(t *testing.T) {
	mgr := loompty.NewManager()
	dir := t.TempDir()

	id1, _ := mgr.Spawn("proc", dir, []string{"/bin/sh"})
	id2, _ := mgr.Spawn("proc", dir, []string{"/bin/sh"})

	if id1 != id2 {
		t.Fatalf("expected deduplicated process, got %s vs %s", id1, id2)
	}

	mgr.Kill(id1)
}

func TestNaturalExit(t *testing.T) {
	mgr := loompty.NewManager()

	// spawn a shell that exits immediately
	id, err := mgr.Spawn("exit-proc", t.TempDir(), []string{"/bin/sh", "-c", "exit 0"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// wait for the reaper goroutine
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !mgr.IsAlive(id) {
			return // success
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected process to be reaped after natural exit")
}
