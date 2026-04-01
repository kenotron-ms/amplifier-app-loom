package pty

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	creackpty "github.com/creack/pty"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Process wraps a running PTY process.
type Process struct {
	ID  string     // stable UUID — safe to use in URLs
	Key string     // deduplication key (projectID::worktreePath)
	ptm *os.File   // PTY master fd
	cmd *exec.Cmd
}

// Manager holds active PTY processes indexed by two keys:
//   procs: UUID id → *Process   (for WebSocket lookup by processId)
//   keys:  key    → UUID id     (for deduplication: same worktree = same process)
type Manager struct {
	mu    sync.Mutex
	procs map[string]*Process // UUID id → process
	keys  map[string]string   // key → UUID id
}

// NewManager returns an initialised Manager.
func NewManager() *Manager {
	return &Manager{
		procs: make(map[string]*Process),
		keys:  make(map[string]string),
	}
}

// Spawn starts a PTY process for key in workDir running argv.
// Returns a clean UUID processId safe for use in URLs.
// If a live process already exists for this key, its ID is returned (deduplication).
func (m *Manager) Spawn(key, workDir string, argv []string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return existing live process for this key.
	if id, ok := m.keys[key]; ok {
		if _, alive := m.procs[id]; alive {
			return id, nil
		}
		delete(m.keys, key)
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptm, err := creackpty.Start(cmd)
	if err != nil {
		return "", fmt.Errorf("pty start: %w", err)
	}

	id := uuid.New().String() // UUID — no slashes, safe in URL paths
	proc := &Process{ID: id, Key: key, ptm: ptm, cmd: cmd}
	m.procs[id] = proc
	m.keys[key] = id

	// Reap when process exits naturally.
	go func() {
		cmd.Wait() //nolint:errcheck
		m.mu.Lock()
		delete(m.procs, id)
		delete(m.keys, key)
		m.mu.Unlock()
	}()

	return id, nil
}

// IsAlive reports whether a process with the given UUID id is running.
func (m *Manager) IsAlive(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.procs[id]
	return ok
}

// Resize sends a SIGWINCH to the PTY so the running process reflows its display.
func (m *Manager) Resize(id string, cols, rows uint16) error {
	m.mu.Lock()
	p, ok := m.procs[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("process %s not found", id)
	}
	return creackpty.Setsize(p.ptm, &creackpty.Winsize{Cols: cols, Rows: rows})
}

// Kill terminates the process and removes it from both maps.
func (m *Manager) Kill(id string) error {
	m.mu.Lock()
	p, ok := m.procs[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("process %s not found", id)
	}
	delete(m.procs, id)
	delete(m.keys, p.Key)
	m.mu.Unlock()

	if p.cmd.Process != nil {
		p.cmd.Process.Kill() //nolint:errcheck
	}
	return p.ptm.Close()
}

// ── WebSocket bridge ──────────────────────────────────────────────────────────

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ServeWS upgrades the HTTP connection to WebSocket and bidirectionally
// bridges it to the PTY identified by processID (UUID). Blocks until closed.
func (m *Manager) ServeWS(w http.ResponseWriter, r *http.Request, processID string) {
	m.mu.Lock()
	p, ok := m.procs[processID]
	m.mu.Unlock()
	if !ok {
		http.Error(w, "process not found", http.StatusNotFound)
		return
	}

	// Ensure the PTY reader goroutine exits when this handler returns.
	defer p.ptm.SetReadDeadline(time.Now()) //nolint:errcheck

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// PTY → WebSocket (goroutine)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := p.ptm.Read(buf)
			if err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
	}()

	// WebSocket → PTY stdin (main loop)
	// Messages are either raw keystrokes or a control envelope:
	//   {"type":"resize","cols":N,"rows":N}  → resize the PTY (do NOT forward to process)
	//   anything else                         → raw stdin for the running process
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var ctrl struct {
			Type string `json:"type"`
			Cols uint16 `json:"cols"`
			Rows uint16 `json:"rows"`
		}
		if json.Unmarshal(msg, &ctrl) == nil && ctrl.Type == "resize" {
			creackpty.Setsize(p.ptm, &creackpty.Winsize{Cols: ctrl.Cols, Rows: ctrl.Rows}) //nolint:errcheck
			continue
		}
		if _, err := p.ptm.Write(msg); err != nil {
			return
		}
	}
}
