package scheduler

import (
	"context"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/ms/agent-daemon/internal/types"
)

const cap64 = 64 * 1024 // 64KB accumulator cap

func (r *Runner) execShell(ctx context.Context, job *types.Job, runID string) (output string, exitCode int, err error) {
	command := job.Command
	if job.Shell != nil && job.Shell.Command != "" {
		command = job.Shell.Command
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		// Run through the user's login+interactive shell so the script gets
		// the same PATH and environment the user has in their terminal.
		cmd = exec.CommandContext(ctx, r.userShell(), "-l", "-i", "-c", command)
	}

	if job.CWD != "" {
		cmd.Dir = job.CWD
	}

	// Set HOME so shell init scripts find ~/.zshrc, ~/.nvm, etc.
	// Job-specific RuntimeEnv is overlaid on top.
	home := r.userHome()
	env := os.Environ()
	if home != "" {
		for i, e := range env {
			if strings.HasPrefix(e, "HOME=") {
				env[i] = "HOME=" + home
				break
			}
		}
	}
	for k, v := range job.RuntimeEnv {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	return streamCommand(cmd, r.broadcaster, runID)
}

// streamCommand runs cmd, streaming output chunks to b while accumulating up
// to cap64 bytes for the returned output string. It is shared by all executors.
func streamCommand(cmd *exec.Cmd, b *Broadcaster, runID string) (output string, exitCode int, err error) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", -1, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", -1, err
	}

	if err = cmd.Start(); err != nil {
		return "", -1, err
	}

	var (
		mu  sync.Mutex
		acc strings.Builder
		wg  sync.WaitGroup
	)

	readPipe := func(r io.Reader) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := r.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				b.Write(runID, chunk)
				mu.Lock()
				remaining := cap64 - acc.Len()
				if remaining > 0 {
					if n > remaining {
						acc.WriteString(chunk[:remaining])
					} else {
						acc.WriteString(chunk)
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
	go readPipe(stdoutPipe)
	go readPipe(stderrPipe)

	wg.Wait()

	waitErr := cmd.Wait()

	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	} else if waitErr != nil {
		exitCode = -1
	}

	return acc.String(), exitCode, waitErr
}
