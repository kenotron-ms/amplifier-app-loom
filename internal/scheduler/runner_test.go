package scheduler

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ms/agent-daemon/internal/store"
	"github.com/ms/agent-daemon/internal/types"
)

// waitForStream polls the broadcaster until at least one stream is registered,
// then returns its runID. Fails the test after 5 seconds.
func waitForStream(t *testing.T, b *Broadcaster) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.RLock()
		for k := range b.streams {
			b.mu.RUnlock()
			return k
		}
		b.mu.RUnlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timeout waiting for broadcaster stream to be registered")
	return ""
}

// TestRunnerWiresBroadcaster verifies that after Execute completes:
// - The broadcaster stream is in done state (Subscribe returns done=true)
// - The store has a completed run record
// - The broadcaster received at least one chunk
func TestRunnerWiresBroadcaster(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := store.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	b := NewBroadcaster()
	r := NewRunner(st, b)

	job := &types.Job{
		ID:       "test-job-1",
		Name:     "test-job-1",
		Executor: types.ExecutorShell,
		Shell:    &types.ShellConfig{Command: `echo "hello from runner test"`},
	}

	// Subscribe goroutine — find the runID as soon as it's registered and
	// collect all chunks before Execute completes.
	var (
		gotChunks []string
		subMu     sync.Mutex
		runID     string
		subDone   = make(chan struct{})
	)

	go func() {
		defer close(subDone)
		runID = waitForStream(t, b)

		buffered, ch, done := b.Subscribe(runID)
		if done {
			return
		}
		subMu.Lock()
		gotChunks = append(gotChunks, buffered...)
		subMu.Unlock()

		for chunk := range ch {
			subMu.Lock()
			gotChunks = append(gotChunks, chunk)
			subMu.Unlock()
		}
	}()

	r.Execute(job)

	// Wait for subscriber goroutine to finish (channel closed by Complete).
	select {
	case <-subDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for subscriber goroutine after Execute")
	}

	// After Execute: broadcaster stream must be in done state.
	_, _, done := b.Subscribe(runID)
	if !done {
		t.Error("expected broadcaster stream to be done after Execute completes")
	}

	// Store must have a completed run record.
	runs, err := st.ListRunsForJob(context.Background(), job.ID, 1)
	if err != nil {
		t.Fatalf("failed to list runs: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected at least one run record in store")
	}
	run := runs[0]
	if run.Status != types.RunStatusSuccess {
		t.Errorf("expected run status %s, got %s", types.RunStatusSuccess, run.Status)
	}

	// Broadcaster must have received at least one chunk.
	subMu.Lock()
	chunkCount := len(gotChunks)
	subMu.Unlock()
	if chunkCount == 0 {
		t.Error("expected broadcaster to receive at least one output chunk")
	}
}

// TestRunnerBroadcasterReceivesChunks verifies that during Execute the
// broadcaster's buffer is populated with output chunks.
func TestRunnerBroadcasterReceivesChunks(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := store.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	b := NewBroadcaster()
	r := NewRunner(st, b)

	job := &types.Job{
		ID:       "test-job-2",
		Name:     "test-job-2",
		Executor: types.ExecutorShell,
		Shell:    &types.ShellConfig{Command: `printf "chunk1\nchunk2\n"`},
	}

	var (
		gotChunks []string
		subMu     sync.Mutex
		subDone   = make(chan struct{})
	)

	// Goroutine subscribes before/during execution and collects chunks.
	go func() {
		defer close(subDone)
		runID := waitForStream(t, b)

		buffered, ch, done := b.Subscribe(runID)
		if done {
			return
		}
		subMu.Lock()
		gotChunks = append(gotChunks, buffered...)
		subMu.Unlock()

		for chunk := range ch {
			subMu.Lock()
			gotChunks = append(gotChunks, chunk)
			subMu.Unlock()
		}
	}()

	r.Execute(job)

	// Wait for subscriber goroutine to drain.
	select {
	case <-subDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for subscriber goroutine after Execute")
	}

	// Broadcaster buffer must have been populated with output chunks.
	subMu.Lock()
	chunkCount := len(gotChunks)
	subMu.Unlock()
	if chunkCount == 0 {
		t.Error("expected broadcaster buffer to be populated with output chunks")
	}
}
