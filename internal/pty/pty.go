package pty

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"time"

	creackpty "github.com/creack/pty"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// reAmpSessionID matches amplifier's startup banner line:
//
//	│ Session ID: 65ab7311-33cd-4526-9f5f-ebe6fd40a718  │
var reAmpSessionID = regexp.MustCompile(`Session ID:\s+([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

// outBufMax is the size of the server-side PTY output ring buffer per process.
// Matches the client-side ring buffer (window.__terminalRegistry.buffers).
const outBufMax = 256 * 1024 // 256 KB

// Process wraps a running PTY process.
type Process struct {
	ID  string   // stable UUID — safe to use in URLs
	Key string   // deduplication key (projectID::worktreePath)
	ptm *os.File // PTY master fd
	cmd *exec.Cmd
}

// Manager holds active PTY processes indexed by two keys:
//
//	procs: UUID id → *Process   (for WebSocket lookup by processId)
//	keys:  key    → UUID id     (for deduplication: same worktree = same process)
//
// outBufs is a server-side rolling ring buffer of raw PTY output, keyed by
// process UUID.  It lets a fresh browser tab replay terminal history after a
// page reload — the client sends ?fresh=1 on the WebSocket URL and the server
// replays the buffer before starting the live stream.
type Manager struct {
	mu          sync.Mutex
	procs       map[string]*Process // UUID id → process
	keys        map[string]string   // key → UUID id
	outputSinks sync.Map            // processID → func([]byte)

	outBufsMu sync.RWMutex
	outBufs   map[string][]byte // processID → rolling output bytes
}

// OnOutput registers a callback that receives every chunk of PTY output for
// processID. Returns a cancel function that deregisters the callback.
// Used to scan stdout for the amplifier session ID banner.
func (m *Manager) OnOutput(processID string, cb func([]byte)) (cancel func()) {
	key := processID
	m.outputSinks.Store(key, cb)
	return func() { m.outputSinks.Delete(key) }
}

// ScanForSessionID registers a one-shot output scanner on processID.
// When the amplifier "Session ID: <uuid>" banner line appears, onFound is
// called with the UUID and the scanner is automatically deregistered.
func (m *Manager) ScanForSessionID(processID string, onFound func(ampSessionID string)) {
	var cancel func()
	cancel = m.OnOutput(processID, func(data []byte) {
		matches := reAmpSessionID.FindSubmatch(data)
		if len(matches) < 2 {
			return
		}
		if cancel != nil {
			cancel() // one-shot: deregister immediately
		}
		onFound(string(matches[1]))
	})
}

// NewManager returns an initialised Manager.
func NewManager() *Manager {
	return &Manager{
		procs:   make(map[string]*Process),
		keys:    make(map[string]string),
		outBufs: make(map[string][]byte),
	}
}

// appendOutBuf appends data to the rolling ring buffer for processID,
// trimming from the front if it exceeds outBufMax.
func (m *Manager) appendOutBuf(processID string, data []byte) {
	m.outBufsMu.Lock()
	buf := append(m.outBufs[processID], data...)
	if len(buf) > outBufMax {
		buf = buf[len(buf)-outBufMax:]
	}
	m.outBufs[processID] = buf
	m.outBufsMu.Unlock()
}

// snapshotOutBuf returns a copy of the current ring buffer for processID.
func (m *Manager) snapshotOutBuf(processID string) []byte {
	m.outBufsMu.RLock()
	raw := m.outBufs[processID]
	snap := make([]byte, len(raw))
	copy(snap, raw)
	m.outBufsMu.RUnlock()
	return snap
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

	// Reap when process exits naturally; clear ring buffer when it does.
	go func() {
		cmd.Wait() //nolint:errcheck
		m.mu.Lock()
		delete(m.procs, id)
		delete(m.keys, key)
		m.mu.Unlock()
		// Clear the output buffer — the process is gone, no point keeping history.
		m.outBufsMu.Lock()
		delete(m.outBufs, id)
		m.outBufsMu.Unlock()
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
//
// Query parameters:
//
//	?fresh=1  — client is a fresh page load (tab reopen). The server sends
//	            the ring buffer as the first WS message so the terminal is
//	            restored to its pre-reload state.
//	            Omit (or ?fresh=0) for reconnects within the same page
//	            session to avoid duplicating output already in the client.
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

	// Replay ring buffer for fresh page loads (?fresh=1).
	// Reconnects skip this to avoid duplicating output already in the client.
	if r.URL.Query().Get("fresh") == "1" {
		if snap := m.snapshotOutBuf(processID); len(snap) > 0 {
			conn.WriteMessage(websocket.BinaryMessage, snap) //nolint:errcheck
		}
	}

	// PTY → WebSocket (goroutine)
	// Every chunk is also appended to the ring buffer and forwarded to any
	// registered output scanners (e.g. session-ID capture).
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := p.ptm.Read(buf)
			if err != nil {
				return
			}
			chunk := buf[:n]

			// Maintain server-side ring buffer (for fresh-connect replay).
			m.appendOutBuf(processID, chunk)

			// Tap: notify any registered output scanners.
			if cb, ok := m.outputSinks.Load(processID); ok {
				cb.(func([]byte))(chunk)
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, chunk); err != nil {
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
