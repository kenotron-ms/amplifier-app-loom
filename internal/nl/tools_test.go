package nl

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/ms/amplifier-app-loom/internal/config"
	"github.com/ms/amplifier-app-loom/internal/types"
)

// ── in-memory store ───────────────────────────────────────────────────────────

type memStore struct {
	mu   sync.Mutex
	jobs map[string]*types.Job
}

func newMemStore() *memStore { return &memStore{jobs: make(map[string]*types.Job)} }

func (m *memStore) SaveJob(_ context.Context, job *types.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *job
	m.jobs[job.ID] = &cp
	return nil
}

func (m *memStore) GetJob(_ context.Context, id string) (*types.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *j
	return &cp, nil
}

func (m *memStore) ListJobs(_ context.Context) ([]*types.Job, error)                         { return nil, nil }
func (m *memStore) DeleteJob(_ context.Context, _ string) error                              { return nil }
func (m *memStore) SaveRun(_ context.Context, _ *types.JobRun) error                         { return nil }
func (m *memStore) GetRun(_ context.Context, _ string) (*types.JobRun, error)                { return nil, nil }
func (m *memStore) ListRunsForJob(_ context.Context, _ string, _ int) ([]*types.JobRun, error) {
	return nil, nil
}
func (m *memStore) ListRecentRuns(_ context.Context, _ int) ([]*types.JobRun, error) {
	return nil, nil
}
func (m *memStore) DeleteAllRuns(_ context.Context) error                                   { return nil }
func (m *memStore) AppendChatMessage(_ context.Context, _ types.ChatMessage) error          { return nil }
func (m *memStore) ListChatHistory(_ context.Context) ([]types.ChatMessage, error)          { return nil, nil }
func (m *memStore) ClearChatHistory(_ context.Context) error                                { return nil }
func (m *memStore) GetConfig(_ context.Context) (*config.Config, error)                     { return nil, nil }
func (m *memStore) SaveConfig(_ context.Context, _ *config.Config) error                    { return nil }
func (m *memStore) Close() error                                                             { return nil }

// ── mock scheduler ────────────────────────────────────────────────────────────

type mockScheduler struct {
	mu      sync.Mutex
	removed []string
	added   []*types.Job
}

func (m *mockScheduler) RemoveJob(jobID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removed = append(m.removed, jobID)
}

func (m *mockScheduler) AddJob(job *types.Job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.added = append(m.added, job)
}

// ── Bug 1: trigger_schedule alone must be applied ─────────────────────────────

// TestExecuteUpdateJob_TriggerScheduleAloneIsApplied verifies that sending only
// trigger_schedule (without trigger_type) still updates the job's schedule.
// Before the fix the schedule is gated behind trigger_type != "" so it is
// silently discarded.
func TestExecuteUpdateJob_TriggerScheduleAloneIsApplied(t *testing.T) {
	s := newMemStore()
	sched := &mockScheduler{}
	ctx := context.Background()

	job := &types.Job{
		ID:      "job-1",
		Name:    "my-job",
		Enabled: true,
		Trigger: types.Trigger{
			Type:     types.TriggerCron,
			Schedule: "0 * * * * *",
		},
	}
	if err := s.SaveJob(ctx, job); err != nil {
		t.Fatal(err)
	}

	input, _ := json.Marshal(map[string]interface{}{
		"id":               "job-1",
		"trigger_schedule": "0 */5 * * * *",
		// trigger_type intentionally omitted — the AI sends only what changes
	})

	_, _, err := executeUpdateJob(ctx, s, sched, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, _ := s.GetJob(ctx, "job-1")
	if updated.Trigger.Schedule != "0 */5 * * * *" {
		t.Errorf("schedule not updated: got %q, want %q",
			updated.Trigger.Schedule, "0 */5 * * * *")
	}
	// trigger_type must be left unchanged
	if updated.Trigger.Type != types.TriggerCron {
		t.Errorf("trigger type changed unexpectedly: got %q", updated.Trigger.Type)
	}
}

// ── Bug 2: live scheduler must be notified after DB save ──────────────────────

// TestExecuteUpdateJob_NotifiesScheduler verifies that after saving to the DB,
// executeUpdateJob calls RemoveJob then AddJob on the live scheduler.
// Before the fix neither call happens.
func TestExecuteUpdateJob_NotifiesScheduler(t *testing.T) {
	s := newMemStore()
	sched := &mockScheduler{}
	ctx := context.Background()

	job := &types.Job{
		ID:      "job-2",
		Name:    "another-job",
		Enabled: true,
		Trigger: types.Trigger{Type: types.TriggerCron, Schedule: "0 * * * * *"},
	}
	if err := s.SaveJob(ctx, job); err != nil {
		t.Fatal(err)
	}

	input, _ := json.Marshal(map[string]interface{}{
		"id":   "job-2",
		"name": "renamed-job",
	})

	_, _, err := executeUpdateJob(ctx, s, sched, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sched.mu.Lock()
	defer sched.mu.Unlock()

	if len(sched.removed) != 1 || sched.removed[0] != "job-2" {
		t.Errorf("expected RemoveJob(%q), got %v", "job-2", sched.removed)
	}
	if len(sched.added) != 1 || sched.added[0].ID != "job-2" {
		t.Errorf("expected AddJob with ID %q, got %v", "job-2", sched.added)
	}
}
