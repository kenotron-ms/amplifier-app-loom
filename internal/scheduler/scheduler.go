package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/ms/amplifier-app-loom/internal/queue"
	"github.com/ms/amplifier-app-loom/internal/store"
	"github.com/ms/amplifier-app-loom/internal/types"
)

// Scheduler manages all job triggers and dispatches to the BoundedQueue.
type Scheduler struct {
	store  store.Store
	queue  *queue.BoundedQueue
	runner *Runner

	cron    *cron.Cron
	cronIDs map[string]cron.EntryID // jobID → cron entry
	loops   map[string]context.CancelFunc
	mu      sync.Mutex

	paused atomic.Bool
	ctx    context.Context
	cancel context.CancelFunc
}

func New(s store.Store, q *queue.BoundedQueue) *Scheduler {
	return &Scheduler{
		store:   s,
		queue:   q,
		runner:  NewRunner(s, nil, nil), // broadcaster/userCtx wired in daemon.go
		cronIDs: make(map[string]cron.EntryID),
		loops:   make(map[string]context.CancelFunc),
		cron: cron.New(
			cron.WithSeconds(),
			cron.WithLogger(cron.DiscardLogger),
		),
	}
}

// Start loads all enabled jobs and begins scheduling. Respects pause state from store.
func (s *Scheduler) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	cfg, err := s.store.GetConfig(ctx)
	if err == nil && cfg.Paused {
		s.paused.Store(true)
	}

	if err := s.loadJobs(ctx); err != nil {
		return err
	}
	s.cron.Start()
	return nil
}

// Stop halts all scheduling.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.cron.Stop()
	s.mu.Lock()
	for _, cancelLoop := range s.loops {
		cancelLoop()
	}
	s.mu.Unlock()
}

// Pause prevents new job dispatches (running jobs continue).
func (s *Scheduler) Pause() { s.paused.Store(true) }

// Resume re-enables job dispatching.
func (s *Scheduler) Resume() { s.paused.Store(false) }

// IsPaused returns true if the scheduler is currently paused.
func (s *Scheduler) IsPaused() bool { return s.paused.Load() }

// Reload re-reads all jobs from store and updates the schedule.
func (s *Scheduler) Reload() error {
	return s.loadJobs(s.ctx)
}

// TriggerNow submits a job for immediate execution regardless of schedule.
func (s *Scheduler) TriggerNow(jobID string) error {
	job, err := s.store.GetJob(s.ctx, jobID)
	if err != nil {
		return err
	}
	s.dispatch(job)
	return nil
}

// TriggerWithEnv dispatches a job with pre-set RuntimeEnv (e.g., from connector sync).
// The job must already have RuntimeEnv populated by the caller.
func (s *Scheduler) TriggerWithEnv(job *types.Job) {
	s.dispatch(job)
}

// AddJob registers a single job in the scheduler.
func (s *Scheduler) AddJob(job *types.Job) {
	if !job.Enabled {
		return
	}
	s.removeJob(job.ID)

	switch job.Trigger.Type {
	case types.TriggerCron:
		id, err := s.cron.AddFunc(job.Trigger.Schedule, s.makeDispatcher(job))
		if err != nil {
			slog.Error("invalid cron expression", "job", job.Name, "schedule", job.Trigger.Schedule, "err", err)
			return
		}
		s.mu.Lock()
		s.cronIDs[job.ID] = id
		s.mu.Unlock()

	case types.TriggerLoop:
		d, err := time.ParseDuration(job.Trigger.Schedule)
		if err != nil {
			slog.Error("invalid loop duration", "job", job.Name, "schedule", job.Trigger.Schedule, "err", err)
			return
		}
		loopCtx, cancel := context.WithCancel(s.ctx)
		s.mu.Lock()
		s.loops[job.ID] = cancel
		s.mu.Unlock()
		go s.runLoop(loopCtx, job, d)

	case "immediate", types.TriggerOnce:
		if job.Enabled {
			delay := time.Duration(0)
			if job.Trigger.Schedule != "" {
				if d, err := time.ParseDuration(job.Trigger.Schedule); err == nil {
					delay = d
				} else {
					slog.Error("invalid once delay", "job", job.Name, "schedule", job.Trigger.Schedule, "err", err)
				}
			}
			onceCtx, cancel := context.WithCancel(s.ctx)
			s.mu.Lock()
			s.loops[job.ID] = cancel
			s.mu.Unlock()
			go func() {
				defer func() {
					s.mu.Lock()
					delete(s.loops, job.ID)
					s.mu.Unlock()
				}()
				if delay > 0 {
					select {
					case <-time.After(delay):
					case <-onceCtx.Done():
						return
					}
				}
				// Re-fetch in case job was modified/deleted during delay
				j, err := s.store.GetJob(onceCtx, job.ID)
				if err != nil || !j.Enabled {
					return
				}
				s.dispatch(j)
			}()
		}

	case types.TriggerWatch:
		watchCtx, cancel := context.WithCancel(s.ctx)
		s.mu.Lock()
		s.loops[job.ID] = cancel
		s.mu.Unlock()
		go s.startWatcher(watchCtx, job)
	}
}

// RemoveJob stops scheduling for a job.
func (s *Scheduler) RemoveJob(jobID string) { s.removeJob(jobID) }

// ─── internals ────────────────────────────────────────────────────────────────

func (s *Scheduler) loadJobs(ctx context.Context) error {
	jobs, err := s.store.ListJobs(ctx)
	if err != nil {
		return err
	}

	// Remove all existing scheduled jobs first
	s.mu.Lock()
	for id, entryID := range s.cronIDs {
		s.cron.Remove(entryID)
		delete(s.cronIDs, id)
	}
	for id, cancel := range s.loops {
		cancel()
		delete(s.loops, id)
	}
	s.mu.Unlock()

	for _, job := range jobs {
		s.AddJob(job)
	}
	return nil
}

func (s *Scheduler) removeJob(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, ok := s.cronIDs[jobID]; ok {
		s.cron.Remove(entryID)
		delete(s.cronIDs, jobID)
	}
	if cancel, ok := s.loops[jobID]; ok {
		cancel()
		delete(s.loops, jobID)
	}
}

func (s *Scheduler) makeDispatcher(job *types.Job) func() {
	jobID := job.ID
	return func() {
		j, err := s.store.GetJob(context.Background(), jobID)
		if err != nil {
			return
		}
		s.dispatch(j)
	}
}

func (s *Scheduler) dispatch(job *types.Job) {
	if s.paused.Load() {
		slog.Debug("scheduler paused, skipping job", "job", job.Name)
		return
	}
	if !job.Enabled {
		return
	}
	submitted := s.queue.Submit(job)
	if !submitted {
		slog.Warn("job skipped (already running or queue full)", "job", job.Name)
	}
}

func (s *Scheduler) runLoop(ctx context.Context, job *types.Job, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Re-fetch job in case it was updated
			j, err := s.store.GetJob(ctx, job.ID)
			if err != nil || !j.Enabled {
				return
			}
			s.dispatch(j)
		}
	}
}
