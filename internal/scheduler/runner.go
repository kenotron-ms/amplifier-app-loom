package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/ms/agent-daemon/internal/store"
	"github.com/ms/agent-daemon/internal/types"
)

const maxOutputBytes = 64 * 1024 // 64KB cap on stored output

type Runner struct {
	store       store.Store
	broadcaster *Broadcaster
}

func NewRunner(s store.Store, b *Broadcaster) *Runner {
	return &Runner{store: s, broadcaster: b}
}

// Execute dispatches to the correct executor based on job type.
func (r *Runner) Execute(job *types.Job) {
	maxRetries := job.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			time.Sleep(backoff)
		}
		done := r.runAttempt(job, attempt+1)
		if done {
			return
		}
	}
}

// runAttempt runs one attempt. Returns true if we should stop retrying.
func (r *Runner) runAttempt(job *types.Job, attempt int) (stopRetrying bool) {
	ctx := context.Background()
	var cancel context.CancelFunc
	if job.Timeout != "" {
		if d, err := time.ParseDuration(job.Timeout); err == nil && d > 0 {
			ctx, cancel = context.WithTimeout(ctx, d)
			defer cancel()
		}
	}

	run := &types.JobRun{
		ID:        uuid.New().String(),
		JobID:     job.ID,
		JobName:   job.Name,
		StartedAt: time.Now(),
		Status:    types.RunStatusRunning,
		Attempt:   attempt,
	}
	_ = r.store.SaveRun(context.Background(), run)

	r.broadcaster.Register(run.ID)
	defer r.broadcaster.Complete(run.ID)

	slog.Info("job starting", "job", job.Name, "executor", job.ResolvedExecutor(), "attempt", attempt)

	var output string
	var exitCode int
	var runErr error

	switch job.ResolvedExecutor() {
	case types.ExecutorClaudeCode:
		output, exitCode, runErr = execClaudeCode(ctx, job, r.broadcaster, run.ID)
	case types.ExecutorAmplifier:
		output, exitCode, runErr = execAmplifier(ctx, job, r.broadcaster, run.ID)
	default: // ExecutorShell + backward compat
		output, exitCode, runErr = execShell(ctx, job, r.broadcaster, run.ID)
	}

	now := time.Now()
	run.EndedAt = &now
	run.Output = capOutput(output)
	run.ExitCode = exitCode

	if ctx.Err() == context.DeadlineExceeded {
		run.Status = types.RunStatusTimeout
		slog.Warn("job timed out", "job", job.Name, "attempt", attempt)
		return true // no retry on timeout
	}
	if runErr != nil {
		run.Status = types.RunStatusFailed
		slog.Warn("job failed", "job", job.Name, "attempt", attempt, "err", runErr)
		_ = r.store.SaveRun(context.Background(), run)
		return false // allow retry
	}

	run.Status = types.RunStatusSuccess
	slog.Info("job succeeded", "job", job.Name, "attempt", attempt)
	_ = r.store.SaveRun(context.Background(), run)
	return true
}

func capOutput(s string) string {
	b := []byte(s)
	if len(b) > maxOutputBytes {
		b = b[len(b)-maxOutputBytes:]
	}
	return string(b)
}
