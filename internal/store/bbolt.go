package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/ms/agent-daemon/internal/config"
	"github.com/ms/agent-daemon/internal/types"
)

var (
	bucketJobs      = []byte("jobs")
	bucketRuns      = []byte("runs")
	bucketRunsByJob = []byte("runs_by_job") // key: jobID/runID → "" (index)
	bucketConfig    = []byte("config")
	bucketChat      = []byte("chat")
	keyConfig       = []byte("cfg")
)

type BoltStore struct {
	db *bolt.DB
}

func Open(path string) (*BoltStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open db at %s: %w", path, err)
	}
	s := &BoltStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *BoltStore) migrate() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bucketJobs, bucketRuns, bucketRunsByJob, bucketConfig, bucketChat} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *BoltStore) Close() error { return s.db.Close() }

// ── Jobs ──────────────────────────────────────────────────────────────────────

func (s *BoltStore) SaveJob(_ context.Context, job *types.Job) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(job)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketJobs).Put([]byte(job.ID), data)
	})
}

func (s *BoltStore) GetJob(_ context.Context, id string) (*types.Job, error) {
	var job types.Job
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketJobs).Get([]byte(id))
		if data == nil {
			return fmt.Errorf("job %s not found", id)
		}
		return json.Unmarshal(data, &job)
	})
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *BoltStore) ListJobs(_ context.Context) ([]*types.Job, error) {
	var jobs []*types.Job
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketJobs).ForEach(func(_, v []byte) error {
			var j types.Job
			if err := json.Unmarshal(v, &j); err != nil {
				return err
			}
			jobs = append(jobs, &j)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
	})
	return jobs, nil
}

func (s *BoltStore) DeleteJob(_ context.Context, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(bucketJobs).Delete([]byte(id)); err != nil {
			return err
		}
		// Clean up run index entries for this job
		b := tx.Bucket(bucketRunsByJob)
		prefix := []byte(id + "/")
		c := b.Cursor()
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

// ── Runs ──────────────────────────────────────────────────────────────────────

func (s *BoltStore) SaveRun(_ context.Context, run *types.JobRun) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(run)
		if err != nil {
			return err
		}
		if err := tx.Bucket(bucketRuns).Put([]byte(run.ID), data); err != nil {
			return err
		}
		indexKey := []byte(run.JobID + "/" + run.ID)
		return tx.Bucket(bucketRunsByJob).Put(indexKey, []byte(run.StartedAt.Format(time.RFC3339Nano)))
	})
}

func (s *BoltStore) GetRun(_ context.Context, id string) (*types.JobRun, error) {
	var run types.JobRun
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketRuns).Get([]byte(id))
		if data == nil {
			return fmt.Errorf("run %s not found", id)
		}
		return json.Unmarshal(data, &run)
	})
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *BoltStore) ListRunsForJob(_ context.Context, jobID string, limit int) ([]*types.JobRun, error) {
	var runs []*types.JobRun
	err := s.db.View(func(tx *bolt.Tx) error {
		prefix := []byte(jobID + "/")
		c := tx.Bucket(bucketRunsByJob).Cursor()
		var keys []string
		for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			// key is "jobID/runID"
			parts := bytes.SplitN(k, []byte("/"), 2)
			if len(parts) == 2 {
				keys = append(keys, string(parts[1]))
			}
		}
		// most recent first
		sort.Sort(sort.Reverse(sort.StringSlice(keys)))
		if limit > 0 && len(keys) > limit {
			keys = keys[:limit]
		}
		rb := tx.Bucket(bucketRuns)
		for _, runID := range keys {
			data := rb.Get([]byte(runID))
			if data == nil {
				continue
			}
			var r types.JobRun
			if err := json.Unmarshal(data, &r); err != nil {
				return err
			}
			runs = append(runs, &r)
		}
		return nil
	})
	return runs, err
}

func (s *BoltStore) ListRecentRuns(_ context.Context, limit int) ([]*types.JobRun, error) {
	var runs []*types.JobRun
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketRuns).ForEach(func(_, v []byte) error {
			var r types.JobRun
			if err := json.Unmarshal(v, &r); err != nil {
				return err
			}
			runs = append(runs, &r)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}

func (s *BoltStore) DeleteAllRuns(_ context.Context) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(bucketRuns); err != nil {
			return err
		}
		if err := tx.DeleteBucket(bucketRunsByJob); err != nil {
			return err
		}
		if _, err := tx.CreateBucket(bucketRuns); err != nil {
			return err
		}
		_, err := tx.CreateBucket(bucketRunsByJob)
		return err
	})
}

// ── Config ────────────────────────────────────────────────────────────────────

func (s *BoltStore) GetConfig(_ context.Context) (*config.Config, error) {
	cfg := config.Defaults()
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketConfig).Get(keyConfig)
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, cfg)
	})
	return cfg, err
}

func (s *BoltStore) SaveConfig(_ context.Context, cfg *config.Config) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(cfg)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketConfig).Put(keyConfig, data)
	})
}

// ── Chat history ──────────────────────────────────────────────────────────────

func (s *BoltStore) AppendChatMessage(_ context.Context, msg types.ChatMessage) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		// Key: timestamp-prefixed so iteration is chronological.
		key := []byte(msg.CreatedAt.UTC().Format(time.RFC3339Nano) + "/" + msg.ID)
		return tx.Bucket(bucketChat).Put(key, data)
	})
}

func (s *BoltStore) ListChatHistory(_ context.Context) ([]types.ChatMessage, error) {
	var msgs []types.ChatMessage
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketChat).ForEach(func(_, v []byte) error {
			var m types.ChatMessage
			if err := json.Unmarshal(v, &m); err != nil {
				return err
			}
			msgs = append(msgs, m)
			return nil
		})
	})
	return msgs, err
}

func (s *BoltStore) ClearChatHistory(_ context.Context) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(bucketChat); err != nil {
			return err
		}
		_, err := tx.CreateBucket(bucketChat)
		return err
	})
}
