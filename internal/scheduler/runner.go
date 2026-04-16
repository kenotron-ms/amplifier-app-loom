package scheduler

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/store"
	"github.com/ms/amplifier-app-loom/internal/types"
)

const maxOutputBytes = 64 * 1024 // 64KB cap on stored output

type Runner struct {
	store       store.Store
	broadcaster *Broadcaster
	userCtx     *config.UserContext
	runCancels  sync.Map // runID → context.CancelFunc
}

func NewRunner(s store.Store, b *Broadcaster, userCtx *config.UserContext) *Runner {
	return &Runner{store: s, broadcaster: b, userCtx: userCtx}
}

// CancelRun cancels the in-flight run with the given ID.
// Returns true if the run was found and cancelled, false if it was not running.
func (r *Runner) CancelRun(runID string) bool {
	if v, ok := r.runCancels.Load(runID); ok {
		v.(context.CancelFunc)()
		return true
	}
	return false
}

// userShell returns the shell captured at install time, falling back to $SHELL.
func (r *Runner) userShell() string {
	if r.userCtx != nil && r.userCtx.Shell != "" {
		return r.userCtx.Shell
	}
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/zsh"
}

// userHome returns the home directory captured at install time.
func (r *Runner) userHome() string {
	if r.userCtx != nil && r.userCtx.HomeDir != "" {
		return r.userCtx.HomeDir
	}
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

// commandFor runs name through the user's login shell so the binary gets exactly
// the environment the user has in their terminal — full PATH (including nvm,
// brew, pyenv, cargo), auth tokens, NVM_DIR, GOPATH, all of it.
//
// Pattern:  $SHELL -l -i -c 'exec "$@"' -- binary arg1 arg2 …
//
// -l = login shell  → sources /etc/profile, ~/.zprofile, ~/.profile
// -i = interactive  → sources ~/.zshrc / ~/.bashrc (required for nvm, brew shellenv)
// exec "$@"         → replaces the shell with the actual binary after init;
//
//	args are passed positionally so no quoting is needed
//
// We only pre-set HOME so the shell can find ~/.zshrc, ~/.nvm, etc.
// Everything else — PATH, NVM_DIR, auth tokens — the shell sets itself.
func (r *Runner) commandFor(ctx context.Context, name string, args ...string) *exec.Cmd {
	shell := r.userShell()
	home := r.userHome()

	shellArgs := make([]string, 0, len(args)+6)
	shellArgs = append(shellArgs, "-l", "-i", "-c", `exec "$@"`, "--", name)
	shellArgs = append(shellArgs, args...)

	cmd := exec.CommandContext(ctx, shell, shellArgs...)

	if home != "" {
		env := os.Environ()
		for i, e := range env {
			if strings.HasPrefix(e, "HOME=") {
				env[i] = "HOME=" + home
				cmd.Env = env
				return cmd
			}
		}
		cmd.Env = append(env, "HOME="+home)
	}
	return cmd
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
	// Always wrap in a cancellable context so CancelRun can interrupt this run.
	baseCtx, baseCancel := context.WithCancel(context.Background())
	defer baseCancel()

	ctx := baseCtx
	if job.Timeout != "" {
		if d, err := time.ParseDuration(job.Timeout); err == nil && d > 0 {
			var timeoutCancel context.CancelFunc
			ctx, timeoutCancel = context.WithTimeout(baseCtx, d)
			defer timeoutCancel()
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

	// Register the cancel func so CancelRun can reach it by run ID.
	r.runCancels.Store(run.ID, baseCancel)
	defer r.runCancels.Delete(run.ID)

	if r.broadcaster != nil {
		r.broadcaster.Register(run.ID)
		defer r.broadcaster.Complete(run.ID)
	}

	slog.Info("job starting", "job", job.Name, "executor", job.ResolvedExecutor(), "attempt", attempt)

	var output string
	var exitCode int
	var runErr error

	switch job.ResolvedExecutor() {
	case types.ExecutorClaudeCode:
		output, exitCode, runErr = r.execClaudeCode(ctx, job, run.ID)
	case types.ExecutorAmplifier:
		output, exitCode, runErr = r.execAmplifier(ctx, job, run.ID)
	default: // ExecutorShell + backward compat
		output, exitCode, runErr = r.execShell(ctx, job, run.ID)
	}

	now := time.Now()
	run.EndedAt = &now
	if output == "" && runErr != nil {
		output = runErr.Error()
	}
	run.Output = capOutput(output)
	run.ExitCode = exitCode

	// Distinguish user cancellation from timeout from normal failure.
	switch ctx.Err() {
	case context.DeadlineExceeded:
		run.Status = types.RunStatusTimeout
		slog.Warn("job timed out", "job", job.Name, "attempt", attempt)
		_ = r.store.SaveRun(context.Background(), run)
		return true
	case context.Canceled:
		run.Status = types.RunStatusCancelled
		slog.Info("job cancelled", "job", job.Name, "attempt", attempt)
		_ = r.store.SaveRun(context.Background(), run)
		return true // do not retry a cancelled run
	}

	if runErr != nil {
		run.Status = types.RunStatusFailed
		slog.Warn("job failed", "job", job.Name, "attempt", attempt, "err", runErr)
		_ = r.store.SaveRun(context.Background(), run)
		return false
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
