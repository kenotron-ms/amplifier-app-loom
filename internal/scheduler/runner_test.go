package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ms/amplifier-app-loom/internal/store"
	"github.com/ms/amplifier-app-loom/internal/types"
)

// setupRunnerTest creates a temp bolt store, a broadcaster, and a runner.
// The store is automatically closed via t.Cleanup.
func setupRunnerTest(t *testing.T) (*Runner, *Broadcaster, store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	b := NewBroadcaster()
	r := NewRunner(st, b, nil)
	return r, b, st
}

// streamResult holds the output collected from a single broadcaster stream.
type streamResult struct {
	runID  string
	chunks []string
}

// collectChunks launches a goroutine that waits for a stream to be registered
// on b, subscribes, and drains all chunks until the stream is done.
// Returns a result channel (buffered, receives exactly one value) and a done
// channel that closes when the goroutine exits — use the done channel for
// timeout detection, then read the result channel.
func collectChunks(t *testing.T, b *Broadcaster) (<-chan streamResult, <-chan struct{}) {
	t.Helper()
	resultCh := make(chan streamResult, 1)
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		runID := waitForStream(t, b)

		buffered, ch, done := b.Subscribe(runID)
		result := streamResult{runID: runID, chunks: append([]string{}, buffered...)}
		if !done {
			for chunk := range ch {
				result.chunks = append(result.chunks, chunk)
			}
		}
		resultCh <- result
	}()

	return resultCh, doneCh
}

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
	r, b, st := setupRunnerTest(t)

	job := &types.Job{
		ID:       "test-job-1",
		Name:     "test-job-1",
		Executor: types.ExecutorShell,
		Shell:    &types.ShellConfig{Command: `echo "hello from runner test"`},
	}

	resultCh, subDone := collectChunks(t, b)
	r.Execute(job)

	select {
	case <-subDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for subscriber goroutine after Execute")
	}

	result := <-resultCh

	// Broadcaster stream must be in done state.
	_, _, done := b.Subscribe(result.runID)
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
	if runs[0].Status != types.RunStatusSuccess {
		t.Errorf("expected run status %s, got %s", types.RunStatusSuccess, runs[0].Status)
	}

	// Broadcaster must have received at least one chunk.
	if len(result.chunks) == 0 {
		t.Error("expected broadcaster to receive at least one output chunk")
	}
}

// TestRunnerBroadcasterReceivesChunks verifies that during Execute the
// broadcaster's buffer is populated with output chunks.
func TestRunnerBroadcasterReceivesChunks(t *testing.T) {
	r, b, _ := setupRunnerTest(t)

	job := &types.Job{
		ID:       "test-job-2",
		Name:     "test-job-2",
		Executor: types.ExecutorShell,
		Shell:    &types.ShellConfig{Command: `printf "chunk1\nchunk2\n"`},
	}

	resultCh, subDone := collectChunks(t, b)
	r.Execute(job)

	select {
	case <-subDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for subscriber goroutine after Execute")
	}

	result := <-resultCh

	// Stream must be in done state after Execute.
	_, _, done := b.Subscribe(result.runID)
	if !done {
		t.Error("expected broadcaster stream to be done after Execute completes")
	}

	// Broadcaster buffer must have been populated with output chunks.
	if len(result.chunks) == 0 {
		t.Error("expected broadcaster buffer to be populated with output chunks")
	}
}
