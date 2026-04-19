package meeting_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ms/amplifier-app-loom/internal/meeting"
)

func TestTranscriber_WritesMarkdown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}
		fmt.Fprint(w, "Hello from the meeting. Action item: ship it.")
	}))
	defer srv.Close()

	dir := t.TempDir()
	recDir := filepath.Join(dir, "recordings")
	os.MkdirAll(recDir, 0o755)
	os.MkdirAll(filepath.Join(dir, "transcripts"), 0o755)
	wavPath := filepath.Join(recDir, "2026-04-18_14-30_teams.wav")
	os.WriteFile(wavPath, []byte("RIFF fake wav data"), 0o644)

	trans := meeting.NewTranscriberWithURL(srv.URL, "test-key")
	cfg := meeting.Config{OutputDir: dir, Model: "whisper-1"}

	mdPath, err := trans.Transcribe(context.Background(), wavPath, cfg)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}

	content, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	body := string(content)
	if !containsStr(body, "Hello from the meeting") {
		t.Errorf("transcript missing expected text:\n%s", body)
	}
	if !containsStr(body, "2026-04-18_14-30_teams") {
		t.Errorf("transcript missing source filename:\n%s", body)
	}
}

func TestTranscriber_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error": {"message": "invalid api key"}}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "recordings"), 0o755)
	os.MkdirAll(filepath.Join(dir, "transcripts"), 0o755)
	wavPath := filepath.Join(dir, "recordings", "test.wav")
	os.WriteFile(wavPath, []byte("RIFF"), 0o644)

	trans := meeting.NewTranscriberWithURL(srv.URL, "bad-key")
	_, err := trans.Transcribe(context.Background(), wavPath, meeting.Config{OutputDir: dir, Model: "whisper-1"})
	if err == nil {
		t.Error("expected error for 401 response, got nil")
	}
}

func containsStr(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}
