package scheduler

import (
	"sync"
	"time"
)

const subscriberChanCap = 256

// runStream holds the per-run buffer, subscriber channels, and completion state.
type runStream struct {
	mu          sync.Mutex
	buffer      []string
	subscribers []chan string
	done        bool
}

// Broadcaster fans out live output chunks to SSE subscribers.
// One instance lives for the lifetime of the daemon; individual run streams
// are registered at run start and cleaned up 60 seconds after completion.
type Broadcaster struct {
	mu      sync.RWMutex
	streams map[string]*runStream
}

// NewBroadcaster creates a new Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		streams: make(map[string]*runStream),
	}
}

// Register creates a stream slot for runID. Must be called before any Write.
func (b *Broadcaster) Register(runID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.streams[runID] = &runStream{}
}

// Write appends chunk to the run's buffer and fans it out to all current
// subscribers via non-blocking send. Writes to unknown or already-removed
// runIDs are silently dropped.
func (b *Broadcaster) Write(runID, chunk string) {
	b.mu.RLock()
	s, ok := b.streams[runID]
	b.mu.RUnlock()
	if !ok {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.done {
		return
	}

	s.buffer = append(s.buffer, chunk)

	for _, ch := range s.subscribers {
		// Non-blocking send: full channel drops the chunk for that subscriber only.
		select {
		case ch <- chunk:
		default:
		}
	}
}

// Subscribe returns a snapshot of buffered chunks and a channel for future
// writes. Returns done=true (and nil channel) if the run is already complete
// or the runID is unknown.
func (b *Broadcaster) Subscribe(runID string) (buffered []string, ch chan string, done bool) {
	b.mu.RLock()
	s, ok := b.streams[runID]
	b.mu.RUnlock()

	if !ok {
		return nil, nil, true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.done {
		return nil, nil, true
	}

	// Snapshot the buffer before adding subscriber for race-free point-in-time view.
	snapshot := make([]string, len(s.buffer))
	copy(snapshot, s.buffer)

	ch = make(chan string, subscriberChanCap)
	s.subscribers = append(s.subscribers, ch)

	return snapshot, ch, false
}

// Complete marks the stream done, closes all subscriber channels, and
// schedules removal 60 seconds later.
func (b *Broadcaster) Complete(runID string) {
	b.mu.RLock()
	s, ok := b.streams[runID]
	b.mu.RUnlock()
	if !ok {
		return
	}

	s.mu.Lock()
	if !s.done {
		s.done = true
		for _, ch := range s.subscribers {
			close(ch)
		}
		s.subscribers = nil
	}
	s.mu.Unlock()

	// Schedule removal 60 seconds after completion.
	time.AfterFunc(60*time.Second, func() {
		b.Remove(runID)
	})
}

// Unsubscribe removes ch from the run's subscriber list and drains any
// pending items from the channel.
func (b *Broadcaster) Unsubscribe(runID string, ch chan string) {
	b.mu.RLock()
	s, ok := b.streams[runID]
	b.mu.RUnlock()
	if !ok {
		return
	}

	s.mu.Lock()
	newSubs := s.subscribers[:0]
	for _, sub := range s.subscribers {
		if sub != ch {
			newSubs = append(newSubs, sub)
		}
	}
	s.subscribers = newSubs
	s.mu.Unlock()

	// Drain any pending items from the channel.
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// Remove deletes the stream if no subscribers remain; reschedules in 30s if
// subscribers are still draining.
func (b *Broadcaster) Remove(runID string) {
	b.mu.RLock()
	s, ok := b.streams[runID]
	b.mu.RUnlock()
	if !ok {
		return
	}

	s.mu.Lock()
	subscriberCount := len(s.subscribers)
	s.mu.Unlock()

	if subscriberCount > 0 {
		// Reschedule in 30s if subscribers are still draining.
		time.AfterFunc(30*time.Second, func() {
			b.Remove(runID)
		})
		return
	}

	b.mu.Lock()
	delete(b.streams, runID)
	b.mu.Unlock()
}
