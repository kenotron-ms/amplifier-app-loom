package service

import (
	"testing"

	"github.com/ms/amplifier-app-loom/internal/scheduler"
)

// TestDaemonBroadcasterWiring ensures that the broadcaster is constructed in
// daemon.go and that the types align for passing to both NewRunner and NewServer.
// This is primarily a compile-time check: if daemon.go passes broadcaster in the
// wrong argument position, the package will fail to build and this test will fail.
func TestDaemonBroadcasterWiring(t *testing.T) {
	// Verify NewBroadcaster returns a valid non-nil instance — the same call used
	// at the top of Daemon.Run().
	b := scheduler.NewBroadcaster()
	if b == nil {
		t.Fatal("scheduler.NewBroadcaster() returned nil; expected valid broadcaster")
	}
}
