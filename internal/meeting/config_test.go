package meeting_test

import (
	"context"
	"os"
	"testing"

	bolt "go.etcd.io/bbolt"

	"github.com/ms/amplifier-app-loom/internal/meeting"
)

func openTestDB(t *testing.T) *bolt.DB {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "meeting-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	db, err := bolt.Open(f.Name(), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestConfig_DefaultsDisabled(t *testing.T) {
	db := openTestDB(t)
	store := meeting.NewConfigStore(db)

	cfg, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cfg.Enabled {
		t.Error("expected Enabled=false by default")
	}
	if cfg.Model != "whisper-1" {
		t.Errorf("expected Model=whisper-1, got %q", cfg.Model)
	}
}

func TestConfig_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	store := meeting.NewConfigStore(db)

	want := meeting.Config{Enabled: true, OutputDir: "/tmp/meetings", Model: "whisper-1"}
	if err := store.Set(context.Background(), want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
