package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/ms/agent-daemon/internal/api"
	"github.com/ms/agent-daemon/internal/config"
	"github.com/ms/agent-daemon/internal/platform"
	"github.com/ms/agent-daemon/internal/queue"
	"github.com/ms/agent-daemon/internal/scheduler"
	"github.com/ms/agent-daemon/internal/store"
	"github.com/ms/agent-daemon/internal/types"
)

// Daemon wires together store, scheduler, queue, and HTTP server.
type Daemon struct {
	cfg       *config.Config
	store     store.Store
	scheduler *scheduler.Scheduler
	queue     *queue.BoundedQueue
	server    *api.Server
	startedAt time.Time
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewDaemon() (*Daemon, error) {
	dbPath := platform.DBPath()
	s, err := store.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}

	cfg, err := s.GetConfig(context.Background())
	if err != nil {
		cfg = config.Defaults()
	}

	// Override anthropic key from env if set
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		cfg.AnthropicKey = key
	}

	return &Daemon{cfg: cfg, store: s}, nil
}

func (d *Daemon) Run() error {
	d.ctx, d.cancel = context.WithCancel(context.Background())
	d.startedAt = time.Now()

	runner := scheduler.NewRunner(d.store)
	executeFunc := func(job *types.Job) {
		runner.Execute(job)
		if job.Trigger.Type == types.TriggerOnce {
			job.Enabled = false
			job.UpdatedAt = time.Now()
			_ = d.store.SaveJob(context.Background(), job)
		}
	}
	jobQueue := queue.New(d.cfg.MaxParallel, d.cfg.QueueSize, executeFunc)
	sched := scheduler.New(d.store, jobQueue)

	d.queue = jobQueue
	d.scheduler = sched

	srv := api.NewServer(d.cfg, d.store, sched, jobQueue, d.startedAt)
	d.server = srv

	if err := sched.Start(d.ctx); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}

	slog.Info("agent-daemon started",
		"port", d.cfg.Port,
		"db", platform.DBPath(),
		"pid", os.Getpid(),
	)

	return srv.Start(fmt.Sprintf("localhost:%d", d.cfg.Port))
}

func (d *Daemon) Shutdown() {
	slog.Info("agent-daemon shutting down")
	if d.cancel != nil {
		d.cancel()
	}
	if d.scheduler != nil {
		d.scheduler.Stop()
	}
	if d.queue != nil {
		d.queue.Stop()
	}
	if d.server != nil {
		d.server.Stop()
	}
	if d.store != nil {
		d.store.Close()
	}
}
