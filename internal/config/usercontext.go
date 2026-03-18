package config

import (
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
)

// CaptureUserContext captures the identity of the currently running user from
// the OS. For sudo installs, it prefers $SUDO_USER (the real invoking user)
// over the effective user (root). Returns nil if the user cannot be determined.
//
// This function is side-effect-free — callers are responsible for saving the
// result to the store.
func CaptureUserContext() *UserContext {
	var u *user.User
	var err error

	// Prefer the real invoking user when running under sudo.
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		u, err = user.Lookup(sudoUser)
	}
	if u == nil {
		u, err = user.Current()
	}
	if err != nil || u == nil {
		return nil
	}

	return &UserContext{
		HomeDir:  u.HomeDir,
		Username: u.Username,
		Shell:    LookupUserShell(u.Username),
		UID:      u.Uid,
	}
}

// LookupUserShell returns the login shell for the given username.
// On macOS it queries Directory Services; on other platforms it parses /etc/passwd.
// Falls back to /bin/zsh (macOS) or /bin/bash (other).
func LookupUserShell(username string) string {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("dscl", ".", "-read",
			"/Users/"+username, "UserShell").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.HasPrefix(line, "UserShell:") {
					if parts := strings.Fields(line); len(parts) >= 2 {
						return parts[1]
					}
				}
			}
		}
	}

	// Linux / fallback: parse /etc/passwd
	if data, err := os.ReadFile("/etc/passwd"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Split(line, ":")
			if len(fields) >= 7 && fields[0] == username {
				return fields[6]
			}
		}
	}

	if runtime.GOOS == "darwin" {
		return "/bin/zsh"
	}
	return "/bin/bash"
}
