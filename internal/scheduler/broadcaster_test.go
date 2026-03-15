package scheduler

import (
	"testing"
	"time"
)

// TestBroadcaster_WriteAndSubscribe — Register, Subscribe, Write, verify chunk arrives on channel.
func TestBroadcaster_WriteAndSubscribe(t *testing.T) {
	b := NewBroadcaster()
	b.Register("run-1")

	_, ch, done := b.Subscribe("run-1")
	if done {
		t.Fatal("expected done=false for active run")
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}

	b.Write("run-1", "hello")

	select {
	case chunk, ok := <-ch:
		if !ok {
			t.Fatal("channel closed unexpectedly")
		}
		if chunk != "hello" {
			t.Fatalf("expected 'hello', got %q", chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for chunk")
	}
}

// TestBroadcaster_BufferReplayOnLateSubscribe — Write 3 chunks before Subscribe, verify all 3 in buffered slice.
func TestBroadcaster_BufferReplayOnLateSubscribe(t *testing.T) {
	b := NewBroadcaster()
	b.Register("run-2")

	b.Write("run-2", "chunk-1")
	b.Write("run-2", "chunk-2")
	b.Write("run-2", "chunk-3")

	buffered, _, done := b.Subscribe("run-2")
	if done {
		t.Fatal("expected done=false for active run")
	}
	if len(buffered) != 3 {
		t.Fatalf("expected 3 buffered chunks, got %d", len(buffered))
	}
	if buffered[0] != "chunk-1" || buffered[1] != "chunk-2" || buffered[2] != "chunk-3" {
		t.Fatalf("unexpected buffer contents: %v", buffered)
	}
}

// TestBroadcaster_CompleteClosesChannel — Complete must close subscriber channels.
func TestBroadcaster_CompleteClosesChannel(t *testing.T) {
	b := NewBroadcaster()
	b.Register("run-3")

	_, ch, done := b.Subscribe("run-3")
	if done {
		t.Fatal("expected done=false")
	}

	b.Complete("run-3")

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}

// TestBroadcaster_SubscribeAfterComplete — Subscribe after Complete returns done=true and nil channel.
func TestBroadcaster_SubscribeAfterComplete(t *testing.T) {
	b := NewBroadcaster()
	b.Register("run-4")
	b.Complete("run-4")

	_, ch, done := b.Subscribe("run-4")
	if !done {
		t.Fatal("expected done=true after Complete")
	}
	if ch != nil {
		t.Fatal("expected nil channel after Complete")
	}
}

// TestBroadcaster_UnsubscribeStopsDelivery — After Unsubscribe, writes must not block or deliver.
func TestBroadcaster_UnsubscribeStopsDelivery(t *testing.T) {
	b := NewBroadcaster()
	b.Register("run-5")

	_, ch, done := b.Subscribe("run-5")
	if done {
		t.Fatal("expected done=false")
	}

	b.Unsubscribe("run-5", ch)

	// Write should not block and the channel should not receive
	b.Write("run-5", "after-unsub")

	// Drain to ensure the channel doesn't receive the item
	select {
	case item, ok := <-ch:
		if ok {
			t.Fatalf("expected channel to be drained/empty after unsubscribe, got %q", item)
		}
		// Channel was drained (closed) - that's fine
	default:
		// Empty channel - correct, no delivery after unsubscribe
	}
}

// TestBroadcaster_WriteUnknownRunID — Write/Subscribe for unregistered run must not panic, returns done=true.
func TestBroadcaster_WriteUnknownRunID(t *testing.T) {
	b := NewBroadcaster()

	// Write to unknown run should not panic
	b.Write("unknown-run", "chunk")

	// Subscribe to unknown run should return done=true, nil channel
	_, ch, done := b.Subscribe("unknown-run")
	if !done {
		t.Fatal("expected done=true for unknown run")
	}
	if ch != nil {
		t.Fatal("expected nil channel for unknown run")
	}
}

// TestBroadcaster_MultipleSubscribers — Two subscribers both receive the same chunk via fan-out.
func TestBroadcaster_MultipleSubscribers(t *testing.T) {
	b := NewBroadcaster()
	b.Register("run-6")

	_, ch1, _ := b.Subscribe("run-6")
	_, ch2, _ := b.Subscribe("run-6")

	b.Write("run-6", "broadcast-chunk")

	for i, ch := range []chan string{ch1, ch2} {
		select {
		case chunk, ok := <-ch:
			if !ok {
				t.Fatalf("subscriber %d: channel closed unexpectedly", i+1)
			}
			if chunk != "broadcast-chunk" {
				t.Fatalf("subscriber %d: expected 'broadcast-chunk', got %q", i+1, chunk)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timeout waiting for chunk", i+1)
		}
	}
}
