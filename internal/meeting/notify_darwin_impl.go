//go:build darwin

package meeting

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// NewNotifier returns an osascript-backed Notifier. No .app bundle required.
func NewNotifier() Notifier { return &osascriptNotifier{} }

type osascriptNotifier struct{}

func (n *osascriptNotifier) Setup() {} // nothing to set up

func (n *osascriptNotifier) MeetingDetected(app string, callback func(bool)) {
	go func() {
		script := `button returned of (display dialog "` + app + ` meeting detected — record and transcribe?" ` +
			`with title "loom" buttons {"Dismiss", "Record & Transcribe"} ` +
			`default button "Record & Transcribe" giving up after 30)`
		out, err := exec.Command("osascript", "-e", script).Output()
		callback(err == nil && strings.TrimSpace(string(out)) == "Record & Transcribe")
	}()
}

func (n *osascriptNotifier) RecordingReady(wavPath string, durationSec int, callback func(bool)) {
	go func() {
		mins := durationSec / 60
		secs := durationSec % 60
		msg := "recording saved"
		if mins > 0 {
			msg += " · " + itoa(mins) + "m " + itoa(secs) + "s"
		}
		script := `button returned of (display dialog "` + msg + `" ` +
			`with title "loom" buttons {"Later", "Transcribe"} ` +
			`default button "Transcribe" giving up after 30)`
		out, err := exec.Command("osascript", "-e", script).Output()
		callback(err == nil && strings.TrimSpace(string(out)) == "Transcribe")
	}()
}

func (n *osascriptNotifier) Transcribing() {
	exec.Command("osascript", "-e",
		`display notification "This usually takes under a minute" with title "loom" subtitle "Transcribing…"`).Run()
}

func (n *osascriptNotifier) TranscriptReady(mdPath string) {
	name := filepath.Base(mdPath)
	script := `button returned of (display dialog "Transcript ready: ` + name + `" ` +
		`with title "loom" buttons {"Close", "Open File"} ` +
		`default button "Open File" giving up after 15)`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err == nil && strings.TrimSpace(string(out)) == "Open File" {
		exec.Command("open", mdPath).Run()
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
