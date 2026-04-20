package meeting_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	sh "github.com/kenotron-ms/side-huddle/bindings/go"

	"github.com/ms/amplifier-app-loom/internal/meeting"
)

// fakeNotifier records calls for test assertions.
type fakeNotifier struct {
	mu         sync.Mutex
	detected   []string
	detectedCB func(bool)
	readyCB    func(bool)
}

func (f *fakeNotifier) Setup() {}
func (f *fakeNotifier) Transcribing() {}
func (f *fakeNotifier) TranscriptReady(_ string) {}

func (f *fakeNotifier) MeetingDetected(app string, cb func(bool)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.detected = append(f.detected, app)
	f.detectedCB = cb
}

func (f *fakeNotifier) RecordingReady(_ string, _ int, cb func(bool)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readyCB = cb
}

func TestService_StartsIdleWhenDisabled(t *testing.T) {
	db := openTestDB(t)
	store := meeting.NewConfigStore(db)
	// default config has Enabled=false
	svc := meeting.NewService(store, &fakeNotifier{}, nil)

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	if svc.State() != meeting.StateIdle {
		t.Errorf("expected StateIdle when disabled, got %v", svc.State())
	}
}

func TestService_CreatesOutputDirs(t *testing.T) {
	db := openTestDB(t)
	store := meeting.NewConfigStore(db)
	dir := t.TempDir()

	cfg := meeting.Config{Enabled: true, OutputDir: dir, Model: "whisper-1"}
	if err := store.Set(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	svc := meeting.NewService(store, &fakeNotifier{}, nil)
	// Use the test hook to skip the real side-huddle listener
	svc.SetListenerFactory(meeting.NoOpListenerFactory)

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	if _, err := os.Stat(filepath.Join(dir, "recordings")); err != nil {
		t.Errorf("recordings dir not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "transcripts")); err != nil {
		t.Errorf("transcripts dir not created: %v", err)
	}
	if svc.State() != meeting.StateMonitoring {
		t.Errorf("expected StateMonitoring, got %v", svc.State())
	}
}

func TestService_SetEnabled_Toggles(t *testing.T) {
	db := openTestDB(t)
	store := meeting.NewConfigStore(db)
	dir := t.TempDir()
	store.Set(context.Background(), meeting.Config{Enabled: false, OutputDir: dir, Model: "whisper-1"})

	svc := meeting.NewService(store, &fakeNotifier{}, nil)
	svc.SetListenerFactory(meeting.NoOpListenerFactory)
	svc.Start(context.Background())
	defer svc.Stop()

	if svc.State() != meeting.StateIdle {
		t.Fatalf("expected idle before enable")
	}

	if err := svc.SetEnabled(context.Background(), true); err != nil {
		t.Fatalf("SetEnabled true: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if svc.State() != meeting.StateMonitoring {
		t.Errorf("expected monitoring after enable, got %v", svc.State())
	}

	if err := svc.SetEnabled(context.Background(), false); err != nil {
		t.Fatalf("SetEnabled false: %v", err)
	}
	if svc.State() != meeting.StateIdle {
		t.Errorf("expected idle after disable, got %v", svc.State())
	}
}

func TestService_HandleEvent_MeetingDetected(t *testing.T) {
	db := openTestDB(t)
	store := meeting.NewConfigStore(db)
	dir := t.TempDir()
	store.Set(context.Background(), meeting.Config{Enabled: true, OutputDir: dir, Model: "whisper-1"})

	notif := &fakeNotifier{}
	svc := meeting.NewService(store, notif, nil)
	svc.SetListenerFactory(meeting.NoOpListenerFactory)
	svc.Start(context.Background())
	defer svc.Stop()

	// Simulate a MeetingDetected event
	svc.HandleEventForTest(&sh.Event{Kind: sh.MeetingDetected, App: "Teams"})

	// Notifier should have been called with the app name
	notif.mu.Lock()
	detected := notif.detected
	notif.mu.Unlock()

	if len(detected) != 1 || detected[0] != "Teams" {
		t.Errorf("expected MeetingDetected for Teams, got %v", detected)
	}
}

func TestService_HandleEvent_RecordingReady_Transcribes(t *testing.T) {
	db := openTestDB(t)
	store := meeting.NewConfigStore(db)
	dir := t.TempDir()
	store.Set(context.Background(), meeting.Config{Enabled: true, OutputDir: dir, Model: "whisper-1"})

	notif := &fakeNotifier{}
	svc := meeting.NewService(store, notif, nil) // nil trans = skips actual transcription
	svc.SetListenerFactory(meeting.NoOpListenerFactory)
	svc.Start(context.Background())
	defer svc.Stop()

	// Simulate RecordingReady event
	wavPath := filepath.Join(dir, "recordings", "test.wav")
	svc.HandleEventForTest(&sh.Event{Kind: sh.RecordingReady, App: "Teams", Path: wavPath})

	// Give the async callback a moment
	time.Sleep(20 * time.Millisecond)

	// RecordingReady callback should have been registered
	notif.mu.Lock()
	cb := notif.readyCB
	notif.mu.Unlock()
	if cb == nil {
		t.Error("expected RecordingReady callback to be set")
	}
}
