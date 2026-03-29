package scheduler

import (
	"strings"
	"testing"

	"github.com/ms/amplifier-app-loom/internal/types"
)

// TestExecShell_StreamsChunksToBroadcaster runs `echo "line one" && echo "line two"`,
// verifies output contains both lines, and verifies broadcaster buffer has chunks after Complete.
func TestExecShell_StreamsChunksToBroadcaster(t *testing.T) {
	b := NewBroadcaster()
	runID := "test-stream-run"
	b.Register(runID)

	// Subscribe before running so we capture all chunks.
	_, subCh, done := b.Subscribe(runID)
	if done {
		t.Fatal("expected done=false before run")
	}

	job := &types.Job{
		Shell: &types.ShellConfig{Command: `echo "line one" && echo "line two"`},
	}

	r := &Runner{broadcaster: b}
	output, exitCode, err := r.execShell(t.Context(), job, runID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exitCode 0, got %d", exitCode)
	}
	if !strings.Contains(output, "line one") {
		t.Errorf("output missing 'line one': %q", output)
	}
	if !strings.Contains(output, "line two") {
		t.Errorf("output missing 'line two': %q", output)
	}

	// Complete the stream and drain the subscriber channel.
	b.Complete(runID)

	var chunks []string
	for chunk := range subCh {
		chunks = append(chunks, chunk)
	}

	if len(chunks) == 0 {
		t.Error("expected broadcaster to have received at least one chunk")
	}
}

// TestExecShell_CapsAccumulatorAt64KB generates >64KB output and verifies the
// returned output string does not exceed 64KB.
func TestExecShell_CapsAccumulatorAt64KB(t *testing.T) {
	b := NewBroadcaster()
	runID := "test-cap-run"
	b.Register(runID)

	// Generate ~70 000 bytes of output (well over the 64KB cap).
	job := &types.Job{
		Shell: &types.ShellConfig{Command: `head -c 70000 /dev/zero | tr '\0' 'a'`},
	}

	r := &Runner{broadcaster: b}
	output, exitCode, err := r.execShell(t.Context(), job, runID)
	b.Complete(runID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exitCode 0, got %d", exitCode)
	}

	if len(output) > cap64 {
		t.Errorf("output length %d exceeds 64KB cap (%d)", len(output), cap64)
	}
}

// TestExecShell_ExitCodeOnFailure runs `exit 42` and verifies a non-nil error
// is returned and exitCode == 42.
func TestExecShell_ExitCodeOnFailure(t *testing.T) {
	b := NewBroadcaster()
	runID := "test-exit-run"
	b.Register(runID)

	job := &types.Job{
		Shell: &types.ShellConfig{Command: `exit 42`},
	}

	r := &Runner{broadcaster: b}
	_, exitCode, err := r.execShell(t.Context(), job, runID)
	b.Complete(runID)

	if err == nil {
		t.Fatal("expected a non-nil error for exit code 42")
	}
	if exitCode != 42 {
		t.Errorf("expected exitCode 42, got %d", exitCode)
	}
}
