# Log Viewer — Phase 1: Backend Implementation Plan

> **Execution:** Use the subagent-driven-development workflow to implement this plan.

**Goal:** Add a real-time output broadcaster and SSE streaming endpoint so the frontend can watch live job output.  
**Architecture:** A new in-memory `Broadcaster` fans live output chunks to SSE subscribers. The three executors swap `CombinedOutput()` for streaming pipes that write to the broadcaster in real time while still accumulating a capped string for DB storage. A new `GET /api/runs/{id}/stream` handler serves buffered + live output over SSE.  
**Tech Stack:** Go 1.25.6, stdlib `net/http`, `sync`, `io`, `testing`

---

## Codebase orientation

Before touching anything, read these four files so the code you write fits the project:

```
internal/scheduler/runner.go        — Runner struct, runAttempt, capOutput
internal/scheduler/exec_shell.go    — current CombinedOutput pattern to replace
internal/api/server.go              — Server struct, NewServer, registerRoutes, writeJSON, writeError
internal/service/daemon.go          — where runner and server are constructed
```

Key facts:
- Module path: `github.com/ms/loom`
- Binary entrypoint: `cmd/loom/main.go`
- Route registration uses Go 1.22+ pattern: `mux.HandleFunc("GET /api/runs/{id}/stream", s.handler)`
- Path variable extraction: `r.PathValue("id")`
- JSON helpers already exist: `writeJSON(w, status, v)` and `writeError(w, status, "static string")`
- `store.Store` interface is in `internal/store/store.go`; needs `SaveRun`, `GetRun`, `ListRecentRuns`
- `types.RunStatus` constants: `RunStatusRunning`, `RunStatusSuccess`, `RunStatusFailed`, `RunStatusTimeout`
- `types.JobRun.StartedAt` is `time.Time` (JSON: `"startedAt"`), `EndedAt` is `*time.Time` (JSON: `"endedAt"`)

---

## Task 1: Broadcaster — write test, implement, verify

**Files:**
- Create: `internal/scheduler/broadcaster.go`
- Create: `internal/scheduler/broadcaster_test.go`

---

### Step 1: Write the failing test

Create `internal/scheduler/broadcaster_test.go` with this exact content:

```go
package scheduler

import (
	"testing"
	"time"
)

// ── TestBroadcaster_WriteAndSubscribe ─────────────────────────────────────────
// Register a run, subscribe, write a chunk, verify it arrives on the channel.

func TestBroadcaster_WriteAndSubscribe(t *testing.T) {
	b := NewBroadcaster()
	b.Register("run-1")

	buf, ch, done := b.Subscribe("run-1")
	if done {
		t.Fatal("expected done=false for active run")
	}
	if len(buf) != 0 {
		t.Fatalf("expected empty buffer before any writes, got %d chunks", len(buf))
	}

	b.Write("run-1", "hello\n")

	select {
	case chunk, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly")
		}
		if chunk != "hello\n" {
			t.Fatalf("expected 'hello\\n', got %q", chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for chunk")
	}
}

// ── TestBroadcaster_BufferReplayOnLateSubscribe ───────────────────────────────
// Write 3 chunks before Subscribe. Late subscriber must receive all 3 in the
// buffered slice, not the channel.

func TestBroadcaster_BufferReplayOnLateSubscribe(t *testing.T) {
	b := NewBroadcaster()
	b.Register("run-2")

	b.Write("run-2", "a")
	b.Write("run-2", "b")
	b.Write("run-2", "c")

	buffered, _, done := b.Subscribe("run-2")
	if done {
		t.Fatal("expected done=false")
	}
	if len(buffered) != 3 {
		t.Fatalf("expected 3 buffered chunks, got %d", len(buffered))
	}
	if buffered[0] != "a" || buffered[1] != "b" || buffered[2] != "c" {
		t.Fatalf("unexpected buffer contents: %v", buffered)
	}
}

// ── TestBroadcaster_CompleteClosesChannel ─────────────────────────────────────
// Complete must close all subscriber channels so a range/select loop exits.

func TestBroadcaster_CompleteClosesChannel(t *testing.T) {
	b := NewBroadcaster()
	b.Register("run-3")

	_, ch, _ := b.Subscribe("run-3")
	b.Complete("run-3")

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed, got a value instead")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel to close")
	}
}

// ── TestBroadcaster_SubscribeAfterComplete ────────────────────────────────────
// Once Complete has been called, Subscribe must return done=true immediately.

func TestBroadcaster_SubscribeAfterComplete(t *testing.T) {
	b := NewBroadcaster()
	b.Register("run-4")
	b.Write("run-4", "output")
	b.Complete("run-4")

	_, ch, done := b.Subscribe("run-4")
	if !done {
		t.Fatal("expected done=true after Complete")
	}
	if ch != nil {
		t.Fatal("expected nil channel when done=true")
	}
}

// ── TestBroadcaster_UnsubscribeStopsDelivery ──────────────────────────────────
// After Unsubscribe, writes must not block and must not deliver to the old ch.

func TestBroadcaster_UnsubscribeStopsDelivery(t *testing.T) {
	b := NewBroadcaster()
	b.Register("run-5")

	_, ch, _ := b.Subscribe("run-5")
	b.Unsubscribe("run-5", ch)

	// Write should not panic and should not deliver to ch.
	b.Write("run-5", "invisible")

	select {
	case v, ok := <-ch:
		// ch was drained by Unsubscribe; if there's a stale value it was pre-drain.
		// A nil channel would also be fine. Just ensure no new data arrives.
		if ok {
			t.Fatalf("received unexpected value after unsubscribe: %q", v)
		}
	default:
		// nothing in channel — correct
	}
}

// ── TestBroadcaster_WriteUnknownRunID ─────────────────────────────────────────
// Write/Subscribe for an unregistered run ID must not panic.

func TestBroadcaster_WriteUnknownRunID(t *testing.T) {
	b := NewBroadcaster()
	// no Register — should be silently ignored
	b.Write("ghost-run", "data")

	_, ch, done := b.Subscribe("ghost-run")
	if !done {
		t.Fatal("expected done=true for unknown runID")
	}
	if ch != nil {
		t.Fatal("expected nil channel for unknown runID")
	}
}

// ── TestBroadcaster_MultipleSubscribers ───────────────────────────────────────
// Two subscribers both receive the same chunk via fan-out.

func TestBroadcaster_MultipleSubscribers(t *testing.T) {
	b := NewBroadcaster()
	b.Register("run-6")

	_, ch1, _ := b.Subscribe("run-6")
	_, ch2, _ := b.Subscribe("run-6")

	b.Write("run-6", "broadcast")

	for i, ch := range []chan string{ch1, ch2} {
		select {
		case chunk := <-ch:
			if chunk != "broadcast" {
				t.Fatalf("subscriber %d: expected 'broadcast', got %q", i, chunk)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d timed out", i)
		}
	}
}
```

### Step 2: Run the test to verify it fails

```bash
cd /Users/ken/workspace/ms/loom
go test ./internal/scheduler/... -v -run TestBroadcaster 2>&1 | head -30
```

Expected output: compilation error — `NewBroadcaster` undefined. That confirms the test is wired correctly and we need to implement it.

### Step 3: Implement broadcaster.go

Create `internal/scheduler/broadcaster.go` with this exact content:

```go
package scheduler

import (
	"sync"
	"time"
)

// ── Broadcaster ───────────────────────────────────────────────────────────────
//
// Broadcaster fans out live output chunks to SSE subscribers. One broadcaster
// instance lives for the lifetime of the daemon; individual run streams are
// registered at run start and cleaned up 60 seconds after completion.

const subscriberChanCap = 256

type runStream struct {
	mu          sync.Mutex
	buffer      []string    // all chunks written so far (uncapped — for live replay)
	subscribers []chan string
	done        bool
}

// Broadcaster is the top-level fan-out router. Thread-safe.
type Broadcaster struct {
	mu      sync.RWMutex
	streams map[string]*runStream
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{streams: make(map[string]*runStream)}
}

// ── Lifecycle ─────────────────────────────────────────────────────────────────

// Register creates a stream slot for runID. Must be called before any Write.
func (b *Broadcaster) Register(runID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.streams[runID] = &runStream{}
}

// Complete marks the stream done, closes all subscriber channels, and schedules
// removal 60 seconds later (giving late clients time to drain).
func (b *Broadcaster) Complete(runID string) {
	b.mu.RLock()
	s := b.streams[runID]
	b.mu.RUnlock()
	if s == nil {
		return
	}

	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}
	s.done = true
	for _, ch := range s.subscribers {
		close(ch)
	}
	s.subscribers = nil
	s.mu.Unlock()

	time.AfterFunc(60*time.Second, func() { b.Remove(runID) })
}

// Remove deletes the stream if no subscribers remain. If subscribers are still
// draining it reschedules itself in 30 seconds.
func (b *Broadcaster) Remove(runID string) {
	b.mu.RLock()
	s := b.streams[runID]
	b.mu.RUnlock()
	if s == nil {
		return
	}

	s.mu.Lock()
	remaining := len(s.subscribers)
	s.mu.Unlock()

	if remaining > 0 {
		time.AfterFunc(30*time.Second, func() { b.Remove(runID) })
		return
	}

	b.mu.Lock()
	delete(b.streams, runID)
	b.mu.Unlock()
}

// ── Read / Write ──────────────────────────────────────────────────────────────

// Write appends chunk to the run's buffer and fans it out to all current
// subscribers via non-blocking send (a full channel drops the chunk for that
// subscriber only — the buffer is always written so late subscribers still get
// complete history).
// Writes to unknown or already-removed runIDs are silently dropped.
func (b *Broadcaster) Write(runID, chunk string) {
	b.mu.RLock()
	s := b.streams[runID]
	b.mu.RUnlock()
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return
	}
	s.buffer = append(s.buffer, chunk)
	for _, ch := range s.subscribers {
		select {
		case ch <- chunk:
		default:
			// slow subscriber — drop this chunk for them; buffer remains intact
		}
	}
}

// Subscribe returns a snapshot of buffered chunks and a channel for future
// writes. Returns done=true (and a nil channel) if the run is already complete
// or the runID is unknown — callers should fall back to stored output.
func (b *Broadcaster) Subscribe(runID string) (buffered []string, ch chan string, done bool) {
	b.mu.RLock()
	s := b.streams[runID]
	b.mu.RUnlock()
	if s == nil {
		return nil, nil, true
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return nil, nil, true
	}

	ch = make(chan string, subscriberChanCap)
	// Copy the buffer snapshot before adding subscriber so the caller gets a
	// consistent point-in-time view without races.
	snapshot := make([]string, len(s.buffer))
	copy(snapshot, s.buffer)
	s.subscribers = append(s.subscribers, ch)
	return snapshot, ch, false
}

// Unsubscribe removes ch from the run's subscriber list and drains any pending
// items. Safe to call on unknown runIDs or already-removed channels.
func (b *Broadcaster) Unsubscribe(runID string, ch chan string) {
	b.mu.RLock()
	s := b.streams[runID]
	b.mu.RUnlock()
	if s == nil {
		return
	}

	s.mu.Lock()
	filtered := s.subscribers[:0]
	for _, c := range s.subscribers {
		if c != ch {
			filtered = append(filtered, c)
		}
	}
	s.subscribers = filtered
	s.mu.Unlock()

	// Drain so no goroutine blocks on a send to this channel.
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}
```

### Step 4: Run the tests to verify they pass

```bash
go test ./internal/scheduler/... -v -run TestBroadcaster
```

Expected output:
```
=== RUN   TestBroadcaster_WriteAndSubscribe
--- PASS: TestBroadcaster_WriteAndSubscribe (0.00s)
=== RUN   TestBroadcaster_BufferReplayOnLateSubscribe
--- PASS: TestBroadcaster_BufferReplayOnLateSubscribe (0.00s)
=== RUN   TestBroadcaster_CompleteClosesChannel
--- PASS: TestBroadcaster_CompleteClosesChannel (0.00s)
=== RUN   TestBroadcaster_SubscribeAfterComplete
--- PASS: TestBroadcaster_SubscribeAfterComplete (0.00s)
=== RUN   TestBroadcaster_UnsubscribeStopsDelivery
--- PASS: TestBroadcaster_UnsubscribeStopsDelivery (0.00s)
=== RUN   TestBroadcaster_WriteUnknownRunID
--- PASS: TestBroadcaster_WriteUnknownRunID (0.00s)
=== RUN   TestBroadcaster_MultipleSubscribers
--- PASS: TestBroadcaster_MultipleSubscribers (0.00s)
PASS
ok  	github.com/ms/loom/internal/scheduler
```

All 7 tests must pass. If any fail, fix broadcaster.go before continuing.

### Step 5: Commit

```bash
git add internal/scheduler/broadcaster.go internal/scheduler/broadcaster_test.go
git commit -m "feat: add output broadcaster for SSE fan-out"
```

---

## Task 2: Runner wiring — add broadcaster field, call Register/Complete

**Files:**
- Modify: `internal/scheduler/runner.go`
- Create: `internal/scheduler/runner_test.go`

---

### Step 1: Write the failing test

Create `internal/scheduler/runner_test.go`:

```go
package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/ms/loom/internal/store"
	"github.com/ms/loom/internal/types"
)

// TestRunnerWiresBroadcaster verifies that after Execute completes:
//   - The broadcaster has the run in done state (Subscribe returns done=true)
//   - The store has a completed run record
//   - The broadcaster received at least one chunk of output
func TestRunnerWiresBroadcaster(t *testing.T) {
	// Use a real bolt store in a temp directory.
	dbPath := t.TempDir() + "/test.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	b := NewBroadcaster()
	runner := NewRunner(s, b)

	job := &types.Job{
		ID:       "job-test-1",
		Name:     "test-echo",
		Executor: types.ExecutorShell,
		Shell:    &types.ShellConfig{Command: `echo "hello from runner test"`},
		Enabled:  true,
	}

	// Execute is synchronous — it blocks until the job finishes.
	runner.Execute(job)

	// The run should be complete in the broadcaster.
	// We don't know the run ID ahead of time, so we check the store.
	runs, err := s.ListRecentRuns(context.Background(), 1)
	if err != nil || len(runs) == 0 {
		t.Fatalf("expected at least one run in store after Execute, got err=%v runs=%d", err, len(runs))
	}
	run := runs[0]

	if run.Status != types.RunStatusSuccess {
		t.Fatalf("expected status success, got %q", run.Status)
	}

	// Verify broadcaster treats this run as done.
	_, ch, done := b.Subscribe(run.ID)
	if !done {
		t.Fatalf("expected done=true in broadcaster for completed run %s", run.ID)
	}
	if ch != nil {
		t.Fatal("expected nil channel when done=true")
	}
}

// TestRunnerBroadcasterReceivesChunks verifies that during Execute, the
// broadcaster's buffer for the run ID is populated with output chunks.
// We Subscribe before execution starts (in a goroutine) and collect chunks.
func TestRunnerBroadcasterReceivesChunks(t *testing.T) {
	dbPath := t.TempDir() + "/test2.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	b := NewBroadcaster()
	runner := NewRunner(s, b)

	job := &types.Job{
		ID:       "job-test-2",
		Name:     "test-echo-2",
		Executor: types.ExecutorShell,
		Shell:    &types.ShellConfig{Command: `printf "chunk1\nchunk2\n"`},
		Enabled:  true,
	}

	var receivedChunks []string
	done := make(chan struct{})

	// Poll the broadcaster until Register is called, then subscribe.
	go func() {
		defer close(done)
		// Give runner time to call Register then Subscribe.
		time.Sleep(10 * time.Millisecond)
		runs, _ := s.ListRecentRuns(context.Background(), 1)
		if len(runs) == 0 {
			return // too late, run already finished — check buffer below
		}
		_, ch, isDone := b.Subscribe(runs[0].ID)
		if isDone {
			return
		}
		for chunk := range ch {
			receivedChunks = append(receivedChunks, chunk)
		}
	}()

	runner.Execute(job)
	<-done

	// Whether we caught it live or via buffer: check the store output.
	runs, _ := s.ListRecentRuns(context.Background(), 1)
	if len(runs) == 0 {
		t.Fatal("no runs found after Execute")
	}
	if runs[0].Output == "" {
		t.Fatal("expected non-empty output in store")
	}
}
```

### Step 2: Run the test to verify it fails

```bash
go test ./internal/scheduler/... -v -run TestRunner 2>&1 | head -20
```

Expected: compilation error — `NewRunner` doesn't accept a broadcaster yet.

### Step 3: Modify runner.go

Make these exact changes to `internal/scheduler/runner.go`:

**Change 1:** Update the `Runner` struct and `NewRunner` to accept a broadcaster:

Replace:
```go
type Runner struct {
	store store.Store
}

func NewRunner(s store.Store) *Runner {
	return &Runner{store: s}
}
```

With:
```go
type Runner struct {
	store       store.Store
	broadcaster *Broadcaster
}

func NewRunner(s store.Store, b *Broadcaster) *Runner {
	return &Runner{store: s, broadcaster: b}
}
```

**Change 2:** In `runAttempt`, call `Register` after the initial `SaveRun` and defer `Complete`. Find this block (around line 64):

```go
	_ = r.store.SaveRun(context.Background(), run)

	slog.Info("job starting", "job", job.Name, "executor", job.ResolvedExecutor(), "attempt", attempt)
```

Replace with:

```go
	_ = r.store.SaveRun(context.Background(), run)

	// Register run in broadcaster so live SSE subscribers can attach.
	// Defer Complete so it fires after every return path — success, failed, or
	// timeout — always after the final store.SaveRun.
	r.broadcaster.Register(run.ID)
	defer r.broadcaster.Complete(run.ID)

	slog.Info("job starting", "job", job.Name, "executor", job.ResolvedExecutor(), "attempt", attempt)
```

**Change 3:** Update the three exec call sites to pass broadcaster and runID. Find:

```go
	switch job.ResolvedExecutor() {
	case types.ExecutorClaudeCode:
		output, exitCode, runErr = execClaudeCode(ctx, job)
	case types.ExecutorAmplifier:
		output, exitCode, runErr = execAmplifier(ctx, job)
	default: // ExecutorShell + backward compat
		output, exitCode, runErr = execShell(ctx, job)
	}
```

Replace with:

```go
	switch job.ResolvedExecutor() {
	case types.ExecutorClaudeCode:
		output, exitCode, runErr = execClaudeCode(ctx, job, r.broadcaster, run.ID)
	case types.ExecutorAmplifier:
		output, exitCode, runErr = execAmplifier(ctx, job, r.broadcaster, run.ID)
	default: // ExecutorShell + backward compat
		output, exitCode, runErr = execShell(ctx, job, r.broadcaster, run.ID)
	}
```

### Step 4: Verify the build still breaks on purpose

```bash
go build ./... 2>&1 | head -20
```

Expected: errors about `execShell`, `execClaudeCode`, `execAmplifier` having the wrong number of arguments, AND the `NewRunner` call in `daemon.go` missing the broadcaster argument. These are expected — we'll fix them in Tasks 3–6.

### Step 5: Run only the broadcaster tests (they should still pass)

```bash
go test ./internal/scheduler/... -v -run TestBroadcaster
```

All 7 must still pass. The runner tests will fail to compile until Task 3 completes — that's expected.

### Step 6: Commit the partial wiring

```bash
git add internal/scheduler/runner.go internal/scheduler/runner_test.go
git commit -m "feat: wire broadcaster into Runner — Register/Complete around execution"
```

---

## Task 3: exec_shell streaming — replace CombinedOutput with pipe reads

**Files:**
- Modify: `internal/scheduler/exec_shell.go`
- Create: `internal/scheduler/exec_shell_test.go`

---

### Step 1: Write the failing test

Create `internal/scheduler/exec_shell_test.go`:

```go
package scheduler

import (
	"context"
	"strings"
	"testing"

	"github.com/ms/loom/internal/types"
)

func TestExecShell_StreamsChunksToBroadcaster(t *testing.T) {
	b := NewBroadcaster()
	runID := "exec-shell-test-1"
	b.Register(runID)

	job := &types.Job{
		ID:    "j1",
		Name:  "test-shell",
		Shell: &types.ShellConfig{Command: `echo "line one" && echo "line two"`},
	}

	output, exitCode, err := execShell(context.Background(), job, b, runID)
	if err != nil {
		t.Fatalf("execShell returned error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(output, "line one") || !strings.Contains(output, "line two") {
		t.Fatalf("expected output to contain both lines, got: %q", output)
	}

	// Verify the broadcaster buffer has chunks (run is still "active" — we call
	// Complete manually to check the buffer).
	b.Complete(runID)
	buffered, _, done := b.Subscribe(runID)
	if !done {
		t.Fatal("expected done=true after Complete")
	}
	if len(buffered) == 0 {
		t.Fatal("expected at least one chunk in broadcaster buffer")
	}

	// Reassemble the buffered chunks and verify output is present.
	joined := strings.Join(buffered, "")
	if !strings.Contains(joined, "line one") {
		t.Fatalf("broadcaster buffer missing expected content; got: %q", joined)
	}
}

func TestExecShell_CapsAccumulatorAt64KB(t *testing.T) {
	b := NewBroadcaster()
	runID := "exec-shell-test-2"
	b.Register(runID)

	// Generate more than 64KB of output.
	// `yes` would run forever; use a loop instead.
	job := &types.Job{
		ID:    "j2",
		Name:  "test-cap",
		Shell: &types.ShellConfig{Command: `python3 -c "print('x' * 1000)" | head -80`},
	}

	// Fallback: simple repeated echo if python3 not available.
	// The test only cares that output is capped at 64KB.
	output, _, _ := execShell(context.Background(), job, b, runID)

	const cap64 = 64 * 1024
	if len(output) > cap64 {
		t.Fatalf("output exceeds 64KB cap: %d bytes", len(output))
	}
}

func TestExecShell_ExitCodeOnFailure(t *testing.T) {
	b := NewBroadcaster()
	runID := "exec-shell-test-3"
	b.Register(runID)

	job := &types.Job{
		ID:    "j3",
		Name:  "test-fail",
		Shell: &types.ShellConfig{Command: `exit 42`},
	}

	_, exitCode, err := execShell(context.Background(), job, b, runID)
	if err == nil {
		t.Fatal("expected an error for non-zero exit")
	}
	if exitCode != 42 {
		t.Fatalf("expected exit code 42, got %d", exitCode)
	}
}
```

### Step 2: Run the test to verify it fails

```bash
go test ./internal/scheduler/... -v -run TestExecShell 2>&1 | head -20
```

Expected: compilation error — `execShell` doesn't accept broadcaster/runID arguments yet.

### Step 3: Rewrite exec_shell.go

Replace the entire content of `internal/scheduler/exec_shell.go` with:

```go
package scheduler

import (
	"context"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/ms/loom/internal/types"
)

func execShell(ctx context.Context, job *types.Job, b *Broadcaster, runID string) (output string, exitCode int, err error) {
	// Resolve command: prefer Shell config, fall back to top-level Command field.
	command := job.Command
	if job.Shell != nil && job.Shell.Command != "" {
		command = job.Shell.Command
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	if job.CWD != "" {
		cmd.Dir = job.CWD
	}
	if len(job.RuntimeEnv) > 0 {
		env := os.Environ()
		for k, v := range job.RuntimeEnv {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}

	output, exitCode, err = streamCommand(cmd, b, runID)
	return
}

// ── streamCommand ─────────────────────────────────────────────────────────────
//
// streamCommand runs cmd, streaming all stdout+stderr to the broadcaster while
// accumulating up to 64 KB for the DB. It is used by all three executors.
//
// Returns the accumulated (capped) output string, the process exit code, and
// any execution error. The signature is intentionally identical to the old
// cmd.CombinedOutput() usage so callers don't need to change their error
// handling logic.
func streamCommand(cmd *exec.Cmd, b *Broadcaster, runID string) (output string, exitCode int, err error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", -1, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", -1, err
	}

	if err = cmd.Start(); err != nil {
		return "", -1, err
	}

	const cap64 = 64 * 1024
	var (
		mu    sync.Mutex
		accum strings.Builder
		wg    sync.WaitGroup
	)

	readPipe := func(r io.Reader) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := r.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				b.Write(runID, chunk) // broadcaster write is always uncapped
				mu.Lock()
				if accum.Len() < cap64 {
					remaining := cap64 - accum.Len()
					if len(chunk) <= remaining {
						accum.WriteString(chunk)
					} else {
						accum.WriteString(chunk[:remaining])
					}
				}
				mu.Unlock()
			}
			if readErr != nil {
				break
			}
		}
	}

	wg.Add(2)
	go readPipe(stdout)
	go readPipe(stderr)
	wg.Wait()

	err = cmd.Wait()
	output = accum.String()

	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	} else if err != nil {
		exitCode = -1
	}
	return
}
```

### Step 4: Run the tests to verify they pass

```bash
go test ./internal/scheduler/... -v -run TestExecShell
```

Expected: all 3 tests pass. The runner tests still won't compile (exec_claude_code and exec_amplifier still have old signatures) — that's expected until Tasks 7–8.

### Step 5: Commit

```bash
git add internal/scheduler/exec_shell.go internal/scheduler/exec_shell_test.go
git commit -m "feat: stream exec_shell output via broadcaster pipes"
```

---

## Task 4: SSE handler — create handlers_stream.go

**Files:**
- Create: `internal/api/handlers_stream.go`
- Create: `internal/api/handlers_stream_test.go`

The `Server` struct doesn't have the `broadcaster` field yet (that's Task 5). For now, write the handler implementation in full and the test using a `testServer` helper that bypasses `NewServer`.

---

### Step 1: Write the failing test

Create `internal/api/handlers_stream_test.go`:

```go
package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ms/loom/internal/scheduler"
	"github.com/ms/loom/internal/store"
	"github.com/ms/loom/internal/types"
)

// newTestServer builds a minimal Server with a real bolt store and broadcaster.
// The Server fields added in Task 5 are set directly here for test isolation.
func newTestStreamServer(t *testing.T) (*Server, *scheduler.Broadcaster) {
	t.Helper()
	dbPath := t.TempDir() + "/test.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	b := scheduler.NewBroadcaster()
	srv := &Server{
		store:       s,
		broadcaster: b,
	}
	return srv, b
}

// seedRun inserts a completed run into the store and returns it.
func seedRun(t *testing.T, s store.Store, status types.RunStatus, output string) *types.JobRun {
	t.Helper()
	now := time.Now()
	ended := now.Add(5 * time.Second)
	run := &types.JobRun{
		ID:        "test-run-" + t.Name(),
		JobID:     "job-1",
		JobName:   "test-job",
		StartedAt: now,
		EndedAt:   &ended,
		Status:    status,
		Output:    output,
		Attempt:   1,
	}
	if err := s.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	return run
}

// ── TestStreamRun_CompletedRun ────────────────────────────────────────────────
// A completed (non-running) run must emit one data chunk with the stored output
// then an event:done line, then return.

func TestStreamRun_CompletedRun(t *testing.T) {
	srv, _ := newTestStreamServer(t)
	run := seedRun(t, srv.store, types.RunStatusSuccess, "build complete\n")

	req := httptest.NewRequest("GET", "/api/runs/"+run.ID+"/stream", nil)
	w := httptest.NewRecorder()

	srv.streamRun(w, req)

	body := w.Body.String()

	if !strings.Contains(body, `"chunk"`) {
		t.Errorf("expected data chunk in body; got:\n%s", body)
	}
	if !strings.Contains(body, "build complete") {
		t.Errorf("expected output text in body; got:\n%s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Errorf("expected 'event: done' in body; got:\n%s", body)
	}
	if !strings.Contains(body, `"success"`) {
		t.Errorf("expected status 'success' in done payload; got:\n%s", body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}
}

// ── TestStreamRun_NotFound ────────────────────────────────────────────────────

func TestStreamRun_NotFound(t *testing.T) {
	srv, _ := newTestStreamServer(t)

	req := httptest.NewRequest("GET", "/api/runs/does-not-exist/stream", nil)
	// Override PathValue for the test (Go 1.22 mux sets it, but direct calls don't).
	req.SetPathValue("id", "does-not-exist")
	w := httptest.NewRecorder()

	srv.streamRun(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── TestStreamRun_LiveThenDone ────────────────────────────────────────────────
// A running run should stream buffered chunks and then emit event:done when the
// broadcaster's channel closes.

func TestStreamRun_LiveThenDone(t *testing.T) {
	srv, b := newTestStreamServer(t)

	// Insert a run with status=running into the store.
	run := &types.JobRun{
		ID:        "live-run-1",
		JobID:     "job-1",
		JobName:   "live-job",
		StartedAt: time.Now(),
		Status:    types.RunStatusRunning,
		Attempt:   1,
	}
	if err := srv.store.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("seed running run: %v", err)
	}

	// Register in broadcaster and pre-write one buffered chunk.
	b.Register(run.ID)
	b.Write(run.ID, "starting...\n")

	// We'll drive the SSE handler in a goroutine and collect events.
	pipeR, pipeW := http.NewResponseController(nil), (*httptest.ResponseRecorder)(nil)
	_ = pipeR

	// Use a pipe-based approach: run the handler in a goroutine, cancel context
	// to stop it, collect output.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest("GET", "/api/runs/"+run.ID+"/stream", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("id", run.ID)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.streamRun(w, req)
	}()

	// Give the handler time to read the buffered chunk then send more.
	time.Sleep(50 * time.Millisecond)
	b.Write(run.ID, "step 2\n")

	// Simulate run completion: update store then complete broadcaster.
	now := time.Now()
	run.Status = types.RunStatusSuccess
	run.EndedAt = &now
	_ = srv.store.SaveRun(context.Background(), run)
	b.Complete(run.ID)

	// Handler should exit naturally now.
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not exit after broadcaster.Complete")
	}

	body := w.Body.String()

	// Verify buffered chunk was sent.
	if !strings.Contains(body, "starting...") {
		t.Errorf("expected buffered chunk 'starting...' in body; got:\n%s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Errorf("expected 'event: done' in body; got:\n%s", body)
	}

	// Parse the done payload.
	scanner := bufio.NewScanner(strings.NewReader(body))
	var doneData string
	prevWasDoneEvent := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "event: done" {
			prevWasDoneEvent = true
			continue
		}
		if prevWasDoneEvent && strings.HasPrefix(line, "data: ") {
			doneData = strings.TrimPrefix(line, "data: ")
			break
		}
		prevWasDoneEvent = false
	}
	if doneData == "" {
		t.Fatalf("could not find done data line in:\n%s", body)
	}
	var payload struct {
		Status    string `json:"status"`
		StartedAt string `json:"started_at"`
	}
	if err := json.Unmarshal([]byte(doneData), &payload); err != nil {
		t.Fatalf("parse done payload: %v\nraw: %s", err, doneData)
	}
	if payload.Status != "success" {
		t.Errorf("expected status 'success' in done payload, got %q", payload.Status)
	}
}
```

### Step 2: Run the test to verify it fails

```bash
go test ./internal/api/... -v -run TestStreamRun 2>&1 | head -30
```

Expected: compilation error — `Server` has no `broadcaster` field and `streamRun` doesn't exist yet.

### Step 3: Create handlers_stream.go

Create `internal/api/handlers_stream.go` with this exact content:

```go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ms/loom/internal/types"
)

// ── sseRunDone ────────────────────────────────────────────────────────────────

// sseRunDone is the payload sent in the SSE "done" event.
// Uses snake_case field names to match the spec's JavaScript destructuring:
//
//	const { status, started_at, ended_at } = JSON.parse(e.data);
type sseRunDone struct {
	Status    string `json:"status"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
}

// ── streamRun ─────────────────────────────────────────────────────────────────

// streamRun handles GET /api/runs/{id}/stream.
//
// For completed runs: emits stored output as a single chunk event then done.
// For running runs: replays the broadcaster buffer then streams live until the
// run completes (channel closes) or the client disconnects (context cancels).
func (s *Server) streamRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.store.GetRun(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}

	// ── SSE headers ──────────────────────────────────────────────────────────
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// ── Completed run: serve stored output and exit ───────────────────────────
	if run.Status != types.RunStatusRunning {
		emitChunk(w, run.Output)
		emitDone(w, run)
		flusher.Flush()
		return
	}

	// ── Running run: subscribe and stream ─────────────────────────────────────
	buffered, ch, done := s.broadcaster.Subscribe(id)
	if done {
		// Race: run completed between the store.GetRun call above and now.
		// Reload to get the final output.
		run, err = s.store.GetRun(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		emitChunk(w, run.Output)
		emitDone(w, run)
		flusher.Flush()
		return
	}

	// Replay buffered chunks first so late-joining clients see full history.
	for _, chunk := range buffered {
		emitChunk(w, chunk)
	}
	flusher.Flush()

	// Stream live chunks until done or client disconnects.
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				// Channel closed by Complete — run finished.
				// Reload the run to get final status/timing.
				finalRun, err := s.store.GetRun(r.Context(), id)
				if err != nil {
					finalRun = run // fallback to stale run
				}
				emitDone(w, finalRun)
				flusher.Flush()
				return
			}
			emitChunk(w, chunk)
			flusher.Flush()
		case <-r.Context().Done():
			// Client disconnected — clean up subscription.
			s.broadcaster.Unsubscribe(id, ch)
			return
		}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func emitChunk(w http.ResponseWriter, chunk string) {
	payload, _ := json.Marshal(map[string]string{"chunk": chunk})
	fmt.Fprintf(w, "data: %s\n\n", payload)
}

func emitDone(w http.ResponseWriter, run *types.JobRun) {
	d := sseRunDone{
		Status:    string(run.Status),
		StartedAt: run.StartedAt.Format(time.RFC3339),
	}
	if run.EndedAt != nil {
		d.EndedAt = run.EndedAt.Format(time.RFC3339)
	}
	payload, _ := json.Marshal(d)
	fmt.Fprintf(w, "event: done\ndata: %s\n\n", payload)
}
```

### Step 4: Add the broadcaster field to the Server struct temporarily

The test imports `Server.broadcaster` but the field doesn't exist yet. **Only for the purpose of making the tests compile**, add this field to `Server` in `server.go` now. (Task 5 will do the full wiring with the constructor parameter — this is just the field declaration.)

Edit `internal/api/server.go`. Find the `Server` struct:

```go
type Server struct {
	cfg       *config.Config
	store     store.Store
	scheduler *scheduler.Scheduler
	queue     *queue.BoundedQueue
	startedAt time.Time
	nlClient  nl.NLClient
	nlMu      sync.RWMutex
	httpSrv   *http.Server
}
```

Replace with:

```go
type Server struct {
	cfg         *config.Config
	store       store.Store
	scheduler   *scheduler.Scheduler
	queue       *queue.BoundedQueue
	startedAt   time.Time
	nlClient    nl.NLClient
	nlMu        sync.RWMutex
	httpSrv     *http.Server
	broadcaster *scheduler.Broadcaster
}
```

### Step 5: Run the SSE handler tests

```bash
go test ./internal/api/... -v -run TestStreamRun
```

Expected output:
```
=== RUN   TestStreamRun_CompletedRun
--- PASS: TestStreamRun_CompletedRun (0.00s)
=== RUN   TestStreamRun_NotFound
--- PASS: TestStreamRun_NotFound (0.00s)
=== RUN   TestStreamRun_LiveThenDone
--- PASS: TestStreamRun_LiveThenDone (0.05s)
PASS
```

All 3 must pass. If `TestStreamRun_LiveThenDone` times out, check that `broadcaster.Complete` actually closes the channel (Task 1 tests must pass first).

### Step 6: Commit

```bash
git add internal/api/handlers_stream.go internal/api/handlers_stream_test.go internal/api/server.go
git commit -m "feat: SSE stream handler for GET /api/runs/{id}/stream"
```

---

## Task 5: Server wiring — constructor parameter and route registration

**Files:**
- Modify: `internal/api/server.go`

The `broadcaster` field was already added to the struct in Task 4. Now wire it through the constructor and register the route.

---

### Step 1: Update NewServer to accept the broadcaster

In `internal/api/server.go`, find:

```go
func NewServer(cfg *config.Config, s store.Store, sched *scheduler.Scheduler, q *queue.BoundedQueue, startedAt time.Time) *Server {
	srv := &Server{
		cfg:       cfg,
		store:     s,
		scheduler: sched,
		queue:     q,
		startedAt: startedAt,
	}
	srv.nlClient = nl.NewClientFromConfig(cfg, s)
	return srv
}
```

Replace with:

```go
func NewServer(cfg *config.Config, s store.Store, sched *scheduler.Scheduler, q *queue.BoundedQueue, startedAt time.Time, b *scheduler.Broadcaster) *Server {
	srv := &Server{
		cfg:         cfg,
		store:       s,
		scheduler:   sched,
		queue:       q,
		startedAt:   startedAt,
		broadcaster: b,
	}
	srv.nlClient = nl.NewClientFromConfig(cfg, s)
	return srv
}
```

### Step 2: Register the SSE route

In `registerRoutes`, find the runs section:

```go
	// Runs
	mux.HandleFunc("GET /api/runs", s.listRuns)
	mux.HandleFunc("DELETE /api/runs", s.clearRuns)
	mux.HandleFunc("GET /api/runs/{id}", s.getRun)
	mux.HandleFunc("GET /api/jobs/{id}/runs", s.listJobRuns)
```

Replace with:

```go
	// Runs
	mux.HandleFunc("GET /api/runs", s.listRuns)
	mux.HandleFunc("DELETE /api/runs", s.clearRuns)
	mux.HandleFunc("GET /api/runs/{id}", s.getRun)
	mux.HandleFunc("GET /api/runs/{id}/stream", s.streamRun)
	mux.HandleFunc("GET /api/jobs/{id}/runs", s.listJobRuns)
```

### Step 3: Verify compile — expected failure

```bash
go build ./... 2>&1
```

Expected: one error — `api.NewServer` call in `daemon.go` has too few arguments. That's correct. Fix it in Task 6.

### Step 4: Commit

```bash
git add internal/api/server.go
git commit -m "feat: wire broadcaster into Server constructor, register stream route"
```

---

## Task 6: Daemon wiring — construct broadcaster, pass everywhere

**Files:**
- Modify: `internal/service/daemon.go`

---

### Step 1: Update daemon.go

In `internal/service/daemon.go`, find the `Run()` method's setup section:

```go
	runner := scheduler.NewRunner(d.store)
	executeFunc := func(job *types.Job) {
		runner.Execute(job)
		if job.Trigger.Type == types.TriggerOnce {
			job.Enabled = false
			job.UpdatedAt = time.Now()
			_ = d.store.SaveJob(context.Background(), job)
		}
	}
	jobQueue := queue.New(d.cfg.MaxParallel, d.cfg.QueueSize, executeFunc)
	sched := scheduler.New(d.store, jobQueue)

	d.queue = jobQueue
	d.scheduler = sched

	srv := api.NewServer(d.cfg, d.store, sched, jobQueue, d.startedAt)
```

Replace with:

```go
	broadcaster := scheduler.NewBroadcaster()
	runner := scheduler.NewRunner(d.store, broadcaster)
	executeFunc := func(job *types.Job) {
		runner.Execute(job)
		if job.Trigger.Type == types.TriggerOnce {
			job.Enabled = false
			job.UpdatedAt = time.Now()
			_ = d.store.SaveJob(context.Background(), job)
		}
	}
	jobQueue := queue.New(d.cfg.MaxParallel, d.cfg.QueueSize, executeFunc)
	sched := scheduler.New(d.store, jobQueue)

	d.queue = jobQueue
	d.scheduler = sched

	srv := api.NewServer(d.cfg, d.store, sched, jobQueue, d.startedAt, broadcaster)
```

### Step 2: Verify the build compiles cleanly — excluding exec changes

```bash
go build ./... 2>&1
```

Expected: errors about `execClaudeCode` and `execAmplifier` not accepting the new arguments. This is expected — we haven't updated those files yet. The daemon, server, broadcaster, and runner should all compile. Only the exec signature mismatches remain.

### Step 3: Commit

```bash
git add internal/service/daemon.go
git commit -m "feat: construct broadcaster in Daemon, pass to runner and server"
```

---

## Task 7: exec_claude_code streaming

**Files:**
- Modify: `internal/scheduler/exec_claude_code.go`
- Create: `internal/scheduler/exec_claude_code_test.go`

---

### Step 1: Write the test

Create `internal/scheduler/exec_claude_code_test.go`:

```go
package scheduler

import (
	"context"
	"strings"
	"testing"

	"github.com/ms/loom/internal/types"
)

// TestExecClaudeCode_MissingConfig verifies early validation.
func TestExecClaudeCode_MissingConfig(t *testing.T) {
	b := NewBroadcaster()
	b.Register("cc-test-1")

	job := &types.Job{
		ID:       "j-cc-1",
		Executor: types.ExecutorClaudeCode,
		// ClaudeCode deliberately nil to trigger the config check.
	}

	_, exitCode, err := execClaudeCode(context.Background(), job, b, "cc-test-1")
	if err == nil {
		t.Fatal("expected error for nil ClaudeCode config")
	}
	if exitCode != -1 {
		t.Fatalf("expected exit code -1, got %d", exitCode)
	}
	if !strings.Contains(err.Error(), "prompt") {
		t.Errorf("expected error to mention prompt, got: %v", err)
	}
}
```

> **Note on exec_claude_code integration testing:** The `claude` binary is not available in CI. The test above only covers the config-validation fast path. The streaming behaviour is validated end-to-end in Task 9 (integration test) using a shell job, since the streaming pipe code (`streamCommand`) is shared and already tested in Task 3.

### Step 2: Run the test to confirm failure

```bash
go test ./internal/scheduler/... -v -run TestExecClaudeCode 2>&1 | head -20
```

Expected: compilation error — `execClaudeCode` has wrong arity.

### Step 3: Rewrite exec_claude_code.go

Replace the entire content of `internal/scheduler/exec_claude_code.go`:

```go
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ms/loom/internal/types"
)

func execClaudeCode(ctx context.Context, job *types.Job, b *Broadcaster, runID string) (output string, exitCode int, err error) {
	cfg := job.ClaudeCode
	if cfg == nil || cfg.Prompt == "" {
		return "", -1, fmt.Errorf("claude-code executor requires a prompt")
	}

	steps := append([]string{cfg.Prompt}, cfg.Steps...)
	var sessionID string
	var allOutput strings.Builder

	for i, step := range steps {
		args := buildClaudeArgs(cfg, step, sessionID)
		cmd := exec.CommandContext(ctx, "claude", args...)
		if job.CWD != "" {
			cmd.Dir = job.CWD
		}

		// Write step separator to broadcaster before each step (except the first).
		if i > 0 {
			sep := fmt.Sprintf("\n--- step %d ---\n", i+1)
			b.Write(runID, sep)
			allOutput.WriteString(sep)
		}

		stepOut, stepExit, stepErr := streamCommand(cmd, b, runID)
		allOutput.WriteString(stepOut)

		if stepErr != nil {
			return allOutput.String(), stepExit, stepErr
		}

		// Extract session_id from JSON output to chain steps.
		if sessionID == "" {
			if id := extractSessionID(stepOut); id != "" {
				sessionID = id
			}
		}
	}

	return allOutput.String(), 0, nil
}

func buildClaudeArgs(cfg *types.ClaudeCodeConfig, prompt, sessionID string) []string {
	args := []string{"-p", prompt, "--output-format", "json"}

	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(cfg.MaxTurns))
	}
	if len(cfg.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(cfg.AllowedTools, ","))
	}
	if cfg.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", cfg.AppendSystemPrompt)
	}
	return args
}

func extractSessionID(jsonOutput string) string {
	// Claude outputs may have non-JSON lines (e.g. progress). Find the last JSON object.
	lines := strings.Split(strings.TrimSpace(jsonOutput), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var result struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal([]byte(line), &result); err == nil && result.SessionID != "" {
			return result.SessionID
		}
	}
	return ""
}
```

### Step 4: Run the tests

```bash
go test ./internal/scheduler/... -v -run TestExecClaudeCode
```

Expected: `TestExecClaudeCode_MissingConfig` passes.

### Step 5: Commit

```bash
git add internal/scheduler/exec_claude_code.go internal/scheduler/exec_claude_code_test.go
git commit -m "feat: stream exec_claude_code output via broadcaster pipes"
```

---

## Task 8: exec_amplifier streaming

**Files:**
- Modify: `internal/scheduler/exec_amplifier.go`
- Create: `internal/scheduler/exec_amplifier_test.go`

---

### Step 1: Write the test

Create `internal/scheduler/exec_amplifier_test.go`:

```go
package scheduler

import (
	"context"
	"strings"
	"testing"

	"github.com/ms/loom/internal/types"
)

func TestExecAmplifier_MissingConfig(t *testing.T) {
	b := NewBroadcaster()
	b.Register("amp-test-1")

	job := &types.Job{
		ID:       "j-amp-1",
		Executor: types.ExecutorAmplifier,
		// Amplifier deliberately nil.
	}

	_, exitCode, err := execAmplifier(context.Background(), job, b, "amp-test-1")
	if err == nil {
		t.Fatal("expected error for nil Amplifier config")
	}
	if exitCode != -1 {
		t.Fatalf("expected exit code -1, got %d", exitCode)
	}
	if !strings.Contains(err.Error(), "config") {
		t.Errorf("expected error to mention config, got: %v", err)
	}
}

func TestExecAmplifier_MissingPromptAndRecipe(t *testing.T) {
	b := NewBroadcaster()
	b.Register("amp-test-2")

	job := &types.Job{
		ID:        "j-amp-2",
		Executor:  types.ExecutorAmplifier,
		Amplifier: &types.AmplifierConfig{}, // no prompt, no recipe_path
	}

	_, exitCode, err := execAmplifier(context.Background(), job, b, "amp-test-2")
	if err == nil {
		t.Fatal("expected error for empty Amplifier config")
	}
	if exitCode != -1 {
		t.Fatalf("expected exit code -1, got %d", exitCode)
	}
}
```

### Step 2: Run the test to confirm failure

```bash
go test ./internal/scheduler/... -v -run TestExecAmplifier 2>&1 | head -20
```

Expected: compilation error — `execAmplifier` has wrong arity.

### Step 3: Rewrite exec_amplifier.go

Replace the entire content of `internal/scheduler/exec_amplifier.go`:

```go
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ms/loom/internal/types"
)

func execAmplifier(ctx context.Context, job *types.Job, b *Broadcaster, runID string) (output string, exitCode int, err error) {
	cfg := job.Amplifier
	if cfg == nil {
		return "", -1, fmt.Errorf("amplifier executor requires config")
	}

	if cfg.RecipePath != "" {
		return execAmplifierRecipe(ctx, job, cfg, b, runID)
	}
	return execAmplifierPrompt(ctx, job, cfg, b, runID)
}

// execAmplifierPrompt runs one or more prompt steps via `amplifier run`.
func execAmplifierPrompt(ctx context.Context, job *types.Job, cfg *types.AmplifierConfig, b *Broadcaster, runID string) (output string, exitCode int, err error) {
	if cfg.Prompt == "" {
		return "", -1, fmt.Errorf("amplifier executor requires a prompt or recipe_path")
	}

	steps := append([]string{cfg.Prompt}, cfg.Steps...)
	var sessionID string
	var allOutput strings.Builder

	for i, step := range steps {
		args := buildAmplifierArgs(cfg, step, sessionID)
		cmd := exec.CommandContext(ctx, "amplifier", args...)
		if job.CWD != "" {
			cmd.Dir = job.CWD
		}

		if i > 0 {
			sep := fmt.Sprintf("\n--- step %d ---\n", i+1)
			b.Write(runID, sep)
			allOutput.WriteString(sep)
		}

		stepOut, stepExit, stepErr := streamCommand(cmd, b, runID)
		allOutput.WriteString(stepOut)

		if stepErr != nil {
			return allOutput.String(), stepExit, stepErr
		}

		if sessionID == "" {
			if id := extractAmplifierSessionID(stepOut); id != "" {
				sessionID = id
			}
		}
	}

	return allOutput.String(), 0, nil
}

// execAmplifierRecipe runs a recipe via `amplifier tool invoke recipe_execute`.
func execAmplifierRecipe(ctx context.Context, job *types.Job, cfg *types.AmplifierConfig, b *Broadcaster, runID string) (output string, exitCode int, err error) {
	contextJSON := "{}"
	if len(cfg.Context) > 0 {
		byt, _ := json.Marshal(cfg.Context)
		contextJSON = string(byt)
	}

	args := []string{
		"tool", "invoke", "recipe_execute",
		fmt.Sprintf("recipe_path=%s", cfg.RecipePath),
		fmt.Sprintf("context=%s", contextJSON),
	}
	if cfg.Bundle != "" {
		args = append([]string{"--bundle", cfg.Bundle}, args...)
	}

	cmd := exec.CommandContext(ctx, "amplifier", args...)
	if job.CWD != "" {
		cmd.Dir = job.CWD
	}

	return streamCommand(cmd, b, runID)
}

func buildAmplifierArgs(cfg *types.AmplifierConfig, prompt, sessionID string) []string {
	args := []string{"run", "--output-format", "json"}

	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}
	if cfg.Bundle != "" {
		args = append(args, "--bundle", cfg.Bundle)
	}
	if cfg.Model != "" {
		args = append(args, "-m", cfg.Model)
	}
	args = append(args, prompt)
	return args
}

func extractAmplifierSessionID(jsonOutput string) string {
	lines := strings.Split(strings.TrimSpace(jsonOutput), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var result struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal([]byte(line), &result); err == nil && result.SessionID != "" {
			return result.SessionID
		}
	}
	return ""
}
```

### Step 4: Verify everything compiles and all tests pass

```bash
go build ./...
```

Expected: **zero errors**. This is the first clean build with full wiring.

```bash
go test ./internal/... -v
```

Expected: all scheduler and api tests pass. You should see tests like:
```
--- PASS: TestBroadcaster_WriteAndSubscribe
--- PASS: TestBroadcaster_BufferReplayOnLateSubscribe
--- PASS: TestBroadcaster_CompleteClosesChannel
--- PASS: TestBroadcaster_SubscribeAfterComplete
--- PASS: TestBroadcaster_UnsubscribeStopsDelivery
--- PASS: TestBroadcaster_WriteUnknownRunID
--- PASS: TestBroadcaster_MultipleSubscribers
--- PASS: TestRunnerWiresBroadcaster
--- PASS: TestRunnerBroadcasterReceivesChunks
--- PASS: TestExecShell_StreamsChunksToBroadcaster
--- PASS: TestExecShell_ExitCodeOnFailure
--- PASS: TestExecClaudeCode_MissingConfig
--- PASS: TestExecAmplifier_MissingConfig
--- PASS: TestExecAmplifier_MissingPromptAndRecipe
--- PASS: TestStreamRun_CompletedRun
--- PASS: TestStreamRun_NotFound
--- PASS: TestStreamRun_LiveThenDone
```

If any test fails, fix it before moving on.

### Step 5: Commit

```bash
git add internal/scheduler/exec_amplifier.go internal/scheduler/exec_amplifier_test.go
git commit -m "feat: stream exec_amplifier output via broadcaster pipes"
```

---

## Task 9: Integration validation — build binary and run curl test script

**Files:**
- Create: `scripts/test-sse.sh`

This task validates the entire backend end-to-end with a real running daemon, real SSE subscription, and real output streaming.

---

### Step 1: Create the test script

Create `scripts/test-sse.sh` with this exact content:

```bash
#!/usr/bin/env bash
# scripts/test-sse.sh
#
# End-to-end SSE integration test.
# Requires: the daemon binary built and running, plus curl and jq.
#
# Usage:
#   # Terminal 1 — start the daemon however you normally do:
#   ./loom serve
#
#   # Terminal 2 — run this script:
#   PORT=61017 bash scripts/test-sse.sh
#
# The PORT env var must match the daemon's configured port (default 61017).

set -euo pipefail

PORT="${PORT:-61017}"
BASE="http://localhost:${PORT}"
PASS=0
FAIL=0

# ── helpers ────────────────────────────────────────────────────────────────────

ok()   { echo "  ✓ $*"; PASS=$((PASS + 1)); }
fail() { echo "  ✗ $*"; FAIL=$((FAIL + 1)); }

# Wait for the daemon to be healthy (up to 10 seconds).
wait_healthy() {
  echo "Waiting for daemon at ${BASE}/api/status ..."
  for i in $(seq 1 20); do
    if curl -sf "${BASE}/api/status" > /dev/null 2>&1; then
      echo "  ✓ daemon is healthy"
      return 0
    fi
    sleep 0.5
  done
  echo "  ✗ daemon did not become healthy after 10s"
  exit 1
}

# ── tests ─────────────────────────────────────────────────────────────────────

echo ""
echo "=== SSE Integration Test ==="
echo ""

wait_healthy

# ── Step 1: Create a slow shell job ──────────────────────────────────────────
echo "Step 1: Creating test job..."

JOB_JSON=$(curl -sf -X POST "${BASE}/api/jobs" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "sse-test-job",
    "executor": "shell",
    "shell": { "command": "for i in 1 2 3 4 5; do echo \"step $i\"; sleep 0.3; done" },
    "trigger": { "type": "once" },
    "enabled": true
  }')

JOB_ID=$(echo "${JOB_JSON}" | jq -r '.id')
if [[ -z "${JOB_ID}" || "${JOB_ID}" == "null" ]]; then
  echo "  ✗ Failed to create job. Response: ${JOB_JSON}"
  exit 1
fi
ok "Job created: ${JOB_ID}"

# ── Step 2: Trigger the job ───────────────────────────────────────────────────
echo ""
echo "Step 2: Triggering job..."

curl -sf -X POST "${BASE}/api/jobs/${JOB_ID}/trigger" > /dev/null
ok "Trigger POST accepted"

# Wait briefly for the run to be created in the store.
sleep 0.5

# ── Step 3: Get the run ID ────────────────────────────────────────────────────
echo ""
echo "Step 3: Finding run ID..."

for attempt in $(seq 1 10); do
  RUN_JSON=$(curl -sf "${BASE}/api/runs?limit=5")
  RUN_ID=$(echo "${RUN_JSON}" | jq -r --arg jid "${JOB_ID}" \
    '[.[] | select(.jobId == $jid)] | first | .id // empty')
  if [[ -n "${RUN_ID}" ]]; then
    break
  fi
  sleep 0.5
done

if [[ -z "${RUN_ID}" ]]; then
  fail "Could not find run for job ${JOB_ID}"
  exit 1
fi
ok "Run ID: ${RUN_ID}"

# ── Step 4: Subscribe to SSE stream ──────────────────────────────────────────
echo ""
echo "Step 4: Subscribing to SSE stream (max 15s)..."

SSE_OUTPUT=$(curl -sf -N --no-buffer \
  --max-time 15 \
  "${BASE}/api/runs/${RUN_ID}/stream" 2>&1 || true)

if [[ -z "${SSE_OUTPUT}" ]]; then
  fail "SSE endpoint returned empty response"
  exit 1
fi
ok "SSE connection established and received data"

# ── Step 5: Verify at least one data chunk arrived ───────────────────────────
echo ""
echo "Step 5: Verifying SSE data..."

if echo "${SSE_OUTPUT}" | grep -q '"chunk"'; then
  ok "At least one 'chunk' event received"
else
  fail "No 'chunk' events found in SSE output"
  echo "     Raw SSE output (first 500 chars):"
  echo "${SSE_OUTPUT}" | head -c 500
fi

# ── Step 6: Verify content in chunks ─────────────────────────────────────────
if echo "${SSE_OUTPUT}" | grep -q '"step '; then
  ok "Chunk content contains expected 'step N' output"
else
  fail "Expected 'step N' content not found in chunks"
fi

# ── Step 7: Verify event: done arrived ───────────────────────────────────────
if echo "${SSE_OUTPUT}" | grep -q "^event: done"; then
  ok "'event: done' line received"
else
  fail "'event: done' line missing from SSE output"
fi

# ── Step 8: Verify done payload has status=success ───────────────────────────
DONE_PAYLOAD=$(echo "${SSE_OUTPUT}" | awk '/^event: done/{found=1; next} found && /^data: /{print; exit}' | sed 's/^data: //')

if echo "${DONE_PAYLOAD}" | jq -e '.status == "success"' > /dev/null 2>&1; then
  ok "done payload has status=success"
else
  fail "done payload missing or wrong status. Payload: '${DONE_PAYLOAD}'"
fi

# ── Step 9: Verify done payload has started_at ───────────────────────────────
if echo "${DONE_PAYLOAD}" | jq -e '.started_at | length > 0' > /dev/null 2>&1; then
  ok "done payload has started_at timestamp"
else
  fail "done payload missing started_at. Payload: '${DONE_PAYLOAD}'"
fi

# ── Step 10: Verify completed run serves stored output ───────────────────────
echo ""
echo "Step 10: Verifying completed run SSE (replay from store)..."

STORED_SSE=$(curl -sf -N --no-buffer \
  --max-time 5 \
  "${BASE}/api/runs/${RUN_ID}/stream" 2>&1 || true)

if echo "${STORED_SSE}" | grep -q '"chunk"' && echo "${STORED_SSE}" | grep -q "^event: done"; then
  ok "Completed run SSE correctly serves stored output + done event"
else
  fail "Completed run SSE did not return expected output"
  echo "     Raw stored SSE (first 300 chars):"
  echo "${STORED_SSE}" | head -c 300
fi

# ── Cleanup ───────────────────────────────────────────────────────────────────
echo ""
echo "Cleaning up test job..."
curl -sf -X DELETE "${BASE}/api/jobs/${JOB_ID}" > /dev/null || true
ok "Test job deleted"

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "=== Results: ${PASS} passed, ${FAIL} failed ==="
echo ""

if [[ ${FAIL} -gt 0 ]]; then
  exit 1
fi
```

Make it executable:

```bash
chmod +x scripts/test-sse.sh
```

### Step 2: Build the binary

```bash
go build -o ./loom-test ./cmd/loom
```

Expected: binary built with zero errors at `./loom-test`.

### Step 3: Run the integration test

Start the daemon (use the test binary or your normal dev binary — both work):

```bash
# Terminal 1: start the daemon
./loom-test serve
```

Then in a second terminal (or a new bash block):

```bash
# Terminal 2: run the integration test
PORT=61017 bash scripts/test-sse.sh
```

> **If the daemon uses a different port:** check `~/.config/loom/config.json` or your config file and set `PORT=<that-port>`.

Expected output:
```
=== SSE Integration Test ===

Waiting for daemon at http://localhost:61017/api/status ...
  ✓ daemon is healthy

Step 1: Creating test job...
  ✓ Job created: <uuid>

Step 2: Triggering job...
  ✓ Trigger POST accepted

Step 3: Finding run ID...
  ✓ Run ID: <uuid>

Step 4: Subscribing to SSE stream (max 15s)...
  ✓ SSE connection established and received data

Step 5: Verifying SSE data...
  ✓ At least one 'chunk' event received
  ✓ Chunk content contains expected 'step N' output

Step 7: Verifying done event...
  ✓ 'event: done' line received

Step 8: Verifying done payload...
  ✓ done payload has status=success
  ✓ done payload has started_at timestamp

Step 10: Verifying completed run SSE (replay from store)...
  ✓ Completed run SSE correctly serves stored output + done event

Cleaning up test job...
  ✓ Test job deleted

=== Results: 10 passed, 0 failed ===
```

All 10 checks must pass. If any fail:

- **No chunk events:** Check that `execShell` is writing to the broadcaster (Task 3). Run `go test ./internal/scheduler/... -v -run TestExecShell`.
- **No done event:** Check that `broadcaster.Complete` is being called (Task 2 runner wiring). Verify the defer fires by adding a `slog.Info` temporarily.
- **done payload wrong status:** Check `emitDone` in `handlers_stream.go` — the run must be reloaded from the store before calling it.
- **Completed run SSE doesn't replay:** The `run.Status != RunStatusRunning` branch in `streamRun` must be reached. Verify the status was saved to the store correctly.

### Step 4: Clean up the test binary

```bash
rm ./loom-test
```

### Step 5: Commit everything

```bash
git add scripts/test-sse.sh internal/scheduler/exec_amplifier_test.go
git commit -m "test: add SSE integration validation script"

# Final tag for Phase 1 completion
git tag phase1-backend-complete
```

---

## Phase 1 complete — before moving to Phase 2

Run the full test suite one final time:

```bash
go test ./internal/... -v 2>&1 | tail -30
```

Expected: all tests pass, zero failures.

Run the integration script one more time against the live daemon to confirm:

```bash
PORT=61017 bash scripts/test-sse.sh
```

Expected: `Results: 10 passed, 0 failed`.

Only proceed to Phase 2 once both pass cleanly.
