package scheduler

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/ms/agent-daemon/internal/types"
)

// resolveBinary returns the absolute path to a named binary. When the daemon
// runs as a launchd service it inherits a minimal PATH that omits user install
// locations like ~/.local/bin, so exec.LookPath alone is not sufficient.
func resolveBinary(name string) (string, error) {
	// Fast path: already on the daemon's PATH.
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}

	// Slow path: check common user-install locations.
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "bin", name),
		"/usr/local/bin/" + name,
		"/opt/homebrew/bin/" + name,
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("%s binary not found in PATH or common locations (%s)",
		name, strings.Join(candidates, ", "))
}

const cap64 = 64 * 1024 // 64KB accumulator cap

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

	return streamCommand(cmd, b, runID)
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
				// Always broadcast uncapped.
				b.Write(runID, chunk)
				// Accumulate up to cap64 bytes.
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
