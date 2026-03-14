package store

import (
	"context"

	"github.com/ms/agent-daemon/internal/config"
	"github.com/ms/agent-daemon/internal/types"
)

// Store is the persistence interface. All implementations must be goroutine-safe.
type Store interface {
	// Jobs
	SaveJob(ctx context.Context, job *types.Job) error
	GetJob(ctx context.Context, id string) (*types.Job, error)
	ListJobs(ctx context.Context) ([]*types.Job, error)
	DeleteJob(ctx context.Context, id string) error

	// Runs
	SaveRun(ctx context.Context, run *types.JobRun) error
	GetRun(ctx context.Context, id string) (*types.JobRun, error)
	ListRunsForJob(ctx context.Context, jobID string, limit int) ([]*types.JobRun, error)
	ListRecentRuns(ctx context.Context, limit int) ([]*types.JobRun, error)
	DeleteAllRuns(ctx context.Context) error

	// Chat history
	AppendChatMessage(ctx context.Context, msg types.ChatMessage) error
	ListChatHistory(ctx context.Context) ([]types.ChatMessage, error)
	ClearChatHistory(ctx context.Context) error

	// Config
	GetConfig(ctx context.Context) (*config.Config, error)
	SaveConfig(ctx context.Context, cfg *config.Config) error

	Close() error
}
