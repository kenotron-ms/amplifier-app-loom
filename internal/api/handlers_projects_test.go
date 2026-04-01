package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/ms/amplifier-app-loom/internal/api"
	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/files"
	loompty "github.com/ms/amplifier-app-loom/internal/pty"
	"github.com/ms/amplifier-app-loom/internal/store"
	"github.com/ms/amplifier-app-loom/internal/workspaces"
)

func newTestServer(t *testing.T) *api.Server {
	t.Helper()
	tmp := t.TempDir()

	// workspaces DB — separate file to avoid bbolt file-lock conflict
	wsDB, err := bolt.Open(filepath.Join(tmp, "workspaces.db"), 0600, nil)
	if err != nil {
		t.Fatalf("open bolt db: %v", err)
	}
	t.Cleanup(func() { wsDB.Close() })

	// store DB — separate file
	boltStore, err := store.Open(filepath.Join(tmp, "store.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	ws, err := workspaces.New(wsDB)
	if err != nil {
		t.Fatalf("workspaces.New: %v", err)
	}

	cfg := &config.Config{}
	srv := api.NewServer(cfg, boltStore, nil, nil, time.Now(), nil)
	srv.SetWorkspaces(ws, loompty.NewManager(), files.New(t.TempDir()))
	return srv
}

func TestCreateAndListProjects(t *testing.T) {
	srv := newTestServer(t)

	// POST /api/projects
	body := `{"name":"myproject","path":"/tmp/myproject"}`
	req := httptest.NewRequest("POST", "/api/projects", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// GET /api/projects
	req2 := httptest.NewRequest("GET", "/api/projects", nil)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
	var projects []map[string]any
	json.NewDecoder(w2.Body).Decode(&projects)
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
}
