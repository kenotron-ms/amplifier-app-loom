package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/ms/amplifier-app-loom/internal/api"
	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/mirror"
	"github.com/ms/amplifier-app-loom/internal/platform"
	"github.com/ms/amplifier-app-loom/internal/queue"
	"github.com/ms/amplifier-app-loom/internal/scheduler"
	"github.com/ms/amplifier-app-loom/internal/store"
	"github.com/ms/amplifier-app-loom/internal/types"
	"github.com/ms/amplifier-app-loom/internal/workspaces"
)

// Daemon wires together store, scheduler, queue, mirror, and HTTP server.
type Daemon struct {
	cfg         *config.Config
	store       store.Store
	scheduler   *scheduler.Scheduler
	queue       *queue.BoundedQueue
	runner      *scheduler.Runner
	server      *api.Server
	mirrorStore *mirror.MirrorStore
	syncEngine  *mirror.SyncEngine
	startedAt   time.Time
	ctx         context.Context
	cancel      context.CancelFunc
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

	// Override keys from env if set
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		cfg.AnthropicKey = key
		if cfg.AIProvider == "" {
			cfg.AIProvider = "anthropic"
		}
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		cfg.OpenAIKey = key
		if cfg.AIProvider == "" {
			cfg.AIProvider = "openai"
		}
	}

	return &Daemon{cfg: cfg, store: s}, nil
}

func (d *Daemon) Run() error {
	d.ctx, d.cancel = context.WithCancel(context.Background())
	d.startedAt = time.Now()

	broadcaster := scheduler.NewBroadcaster()
	runner := scheduler.NewRunner(d.store, broadcaster, d.cfg.UserContext)
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
	d.runner = runner

	srv := api.NewServer(d.cfg, d.store, sched, jobQueue, d.startedAt, broadcaster)
	srv.SetRunner(runner)
	d.server = srv

	// Wire up the mirror subsystem (connector/entity sync).
	if boltStore, ok := d.store.(*store.BoltStore); ok {
		ms, err := mirror.NewMirrorStore(boltStore.DB())
		if err != nil {
			slog.Error("failed to init mirror store", "err", err)
		} else {
			d.mirrorStore = ms

			fetchers := map[mirror.FetchMethod]mirror.Fetcher{
				mirror.FetchCommand: mirror.NewCommandFetcher(),
				mirror.FetchHTTP:    mirror.NewHTTPFetcher(),
				mirror.FetchBrowser: mirror.NewBrowserFetcher(""),
			}
			se := mirror.NewSyncEngine(ms, fetchers)

			// When a connector detects a change, dispatch its linked jobs.
			se.OnChange = func(conn *mirror.Connector, entity *mirror.Entity, diff *mirror.DiffResult) {
				diffJSON, _ := mirror.DiffToJSON(diff)
				for _, jobID := range conn.JobIDs {
					job, err := d.store.GetJob(d.ctx, jobID)
					if err != nil || !job.Enabled {
						continue
					}
					job.RuntimeEnv = map[string]string{
						"MIRROR_ENTITY":       entity.Address,
						"MIRROR_CONNECTOR_ID": conn.ID,
						"MIRROR_DIFF_JSON":    string(diffJSON),
					}
					sched.TriggerWithEnv(job)
				}
			}

			d.syncEngine = se
			srv.SetMirror(ms, se)
		}

		// Wire up the workspace subsystem (projects).
		ws, wsErr := workspaces.New(boltStore.DB())
		if wsErr != nil {
			slog.Error("failed to init workspace store", "err", wsErr)
		} else {
			srv.SetWorkspaces(ws)
		}
	}

	if err := sched.Start(d.ctx); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}

	// Start the sync engine after the scheduler is running.
	if d.syncEngine != nil {
		d.syncEngine.Start(d.ctx)
	}

	slog.Info("loom started",
		"addr", fmt.Sprintf("0.0.0.0:%d", d.cfg.Port),
		"db", platform.DBPath(),
		"pid", os.Getpid(),
	)

	return srv.Start(fmt.Sprintf(":%d", d.cfg.Port))
}

func (d *Daemon) Shutdown() {
	slog.Info("loom shutting down")
	if d.cancel != nil {
		d.cancel()
	}
	if d.syncEngine != nil {
		d.syncEngine.Stop()
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
