package scheduler

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"

	"github.com/ms/agent-daemon/internal/types"
)

func execShell(ctx context.Context, job *types.Job) (output string, exitCode int, err error) {
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

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err = cmd.Run()
	output = buf.String()
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	} else if err != nil {
		exitCode = -1
	}
	return
}
