//go:build darwin && cgo

package meeting

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	sh "github.com/kenotron-ms/side-huddle/bindings/go"
)


// listenerFactory creates a side-huddle listener for a given output directory.
// Overridable in tests via SetListenerFactory.
type listenerFactory func(outputDir string) (*sh.Listener, error)

// defaultListenerFactory creates a real side-huddle listener.
func defaultListenerFactory(outputDir string) (*sh.Listener, error) {
	l := sh.New().SetOutputDir(outputDir)
	return l, nil
}

// Service detects meetings, records audio, and triggers transcription.
type Service struct {
	store  *ConfigStore
	notify Notifier
	trans  *Transcriber

	newListener     listenerFactory
	mu              sync.Mutex
	state           State
	listener        *sh.Listener
	recStart        time.Time
	cfg             Config
	screenCapture   sh.PermStatus // updated from PermissionStatus events
}

// NewService creates a Service. trans may be nil (disables transcription step).
func NewService(store *ConfigStore, notify Notifier, trans *Transcriber) *Service {
	return &Service{
		store:       store,
		notify:      notify,
		trans:       trans,
		newListener: defaultListenerFactory,
	}
}

// SetListenerFactory overrides the listener constructor (used in tests).
func (s *Service) SetListenerFactory(f func(string) (*sh.Listener, error)) {
	s.newListener = f
}

// Start reads config and begins monitoring if enabled.
func (s *Service) Start(ctx context.Context) error {
	cfg, err := s.store.Get(ctx)
	if err != nil {
		return fmt.Errorf("meeting: load config: %w", err)
	}
	if !cfg.Enabled {
		slog.Info("meeting: disabled, not starting")
		return nil
	}
	return s.startMonitoring(cfg)
}

// Stop halts monitoring and any active recording.
func (s *Service) Stop() {
	s.mu.Lock()
	l := s.listener
	s.listener = nil
	s.mu.Unlock()

	if l != nil {
		l.Stop()
	}
	s.setState(StateIdle)
	slog.Info("meeting: stopped")
}

// SetEnabled enables or disables the service at runtime and persists the change.
func (s *Service) SetEnabled(ctx context.Context, enabled bool) error {
	cfg, err := s.store.Get(ctx)
	if err != nil {
		return err
	}
	cfg.Enabled = enabled
	if err := s.store.Set(ctx, cfg); err != nil {
		return err
	}
	if enabled && s.State() == StateIdle {
		return s.startMonitoring(cfg)
	}
	if !enabled && s.State() != StateIdle {
		s.Stop()
	}
	return nil
}

// State returns the current state (safe for concurrent use).
func (s *Service) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// ── internal ─────────────────────────────────────────────────────────────────

func (s *Service) startMonitoring(cfg Config) error {
	recDir := filepath.Join(cfg.OutputDir, "recordings")
	if err := os.MkdirAll(recDir, 0o755); err != nil {
		return fmt.Errorf("meeting: mkdir recordings: %w", err)
	}
	transDir := filepath.Join(cfg.OutputDir, "transcripts")
	if err := os.MkdirAll(transDir, 0o755); err != nil {
		return fmt.Errorf("meeting: mkdir transcripts: %w", err)
	}

	l, err := s.newListener(recDir)
	if err != nil {
		return fmt.Errorf("meeting: create listener: %w", err)
	}

	if l != nil {
		l.On(s.handleEvent)
		if err := l.Start(); err != nil {
			return fmt.Errorf("meeting: start listener: %w", err)
		}
	}

	s.mu.Lock()
	s.listener = l
	s.cfg = cfg
	s.mu.Unlock()

	s.setState(StateMonitoring)
	slog.Info("meeting: monitoring started", "dir", recDir)
	return nil
}

func (s *Service) handleEvent(e *sh.Event) {
	// Log every event with all relevant fields.
	slog.Info("meeting: event", "kind", e.Kind, "app", e.App, "path", e.Path,
		"permission", e.Permission, "perm_status", e.PermStatus)

	switch e.Kind {
	case sh.PermissionStatus:
		if e.Permission == sh.ScreenCapture {
			s.mu.Lock()
			s.screenCapture = e.PermStatus
			s.mu.Unlock()
			slog.Info("meeting: screen capture permission", "status", e.PermStatus)
		}

	case sh.MeetingDetected:
		s.mu.Lock()
		sc := s.screenCapture
		s.mu.Unlock()

		if sc == sh.Denied {
			// Screen Recording was explicitly denied — tell the overlay and bail.
			if ov, ok := s.notify.(interface{ ShowPermissionNeeded() }); ok {
				ov.ShowPermissionNeeded()
			}
			return
		}

		s.notify.MeetingDetected(e.App, func(record bool) {
			if record {
				s.mu.Lock()
				l := s.listener
				sc := s.screenCapture
				s.mu.Unlock()

				if sc == sh.Denied {
					if ov, ok := s.notify.(interface{ ShowPermissionNeeded() }); ok {
						ov.ShowPermissionNeeded()
					}
					return
				}

				if l != nil {
					l.Record() // first call triggers macOS mic permission prompt
				}
			}
		})

	case sh.RecordingStarted:
		s.mu.Lock()
		s.recStart = time.Now()
		s.mu.Unlock()
		s.setState(StateRecording)

	case sh.RecordingEnded:
		// Capture stopped; WAV is being finalized on disk.
		// Update overlay immediately — MeetingEnded may have fired 20s ago or
		// just before this, either way recording is now over.
		if s.State() == StateRecording {
			if ov, ok := s.notify.(interface{ ShowSaving() }); ok {
				ov.ShowSaving()
			}
			s.setState(StateMonitoring)
		}

	case sh.RecordingReady:
		// WAV is fully written to disk. e.Path has the file.
		s.mu.Lock()
		dur := int(time.Since(s.recStart).Seconds())
		cfg := s.cfg
		s.mu.Unlock()

		slog.Info("meeting: WAV ready", "path", e.Path, "dur_sec", dur)
		s.notify.RecordingReady(e.Path, dur, func(transcribe bool) {
			if transcribe && s.trans != nil {
				s.setState(StateTranscribing)
				s.notify.Transcribing()
				go s.runTranscription(e.Path, cfg)
			}
		})

	case sh.MeetingEnded:
		// Meeting window closed / mic went quiet for 20s.
		// If still in recording state here (RecordingEnded not yet received),
		// show saving state as a fallback.
		if s.State() == StateRecording {
			if ov, ok := s.notify.(interface{ ShowSaving() }); ok {
				ov.ShowSaving()
			}
		}
		if s.State() != StateTranscribing {
			s.setState(StateMonitoring)
		}

	case sh.Error:
		slog.Error("meeting: side-huddle error", "msg", e.Message)
		// Surface the error in the overlay so it doesn't get stuck.
		if ov, ok := s.notify.(interface{ ShowError(string) }); ok {
			ov.ShowError(e.Message)
		}
		s.setState(StateMonitoring)
	}
}

func (s *Service) runTranscription(wavPath string, cfg Config) {
	ctx := context.Background()
	mdPath, err := s.trans.Transcribe(ctx, wavPath, cfg)
	if err != nil {
		slog.Error("meeting: transcription failed", "err", err)
		s.setState(StateMonitoring)
		return
	}
	slog.Info("meeting: transcript saved", "path", mdPath)
	s.setState(StateMonitoring)
	s.notify.TranscriptReady(mdPath)
}

func (s *Service) setState(state State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
}