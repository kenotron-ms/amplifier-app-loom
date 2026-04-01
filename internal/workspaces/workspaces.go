package workspaces

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

var (
	bucketProjects          = []byte("projects")
	bucketSessions          = []byte("sessions")
	bucketSessionsByProject = []byte("sessions_by_project")
)

// Project is a codebase on disk with one or more worktree sessions.
type Project struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Path           string `json:"path"` // absolute path on disk
	CreatedAt      int64  `json:"createdAt"`
	LastActivityAt int64  `json:"lastActivityAt"`
}

// Session is a git worktree within a project, backed by a persistent PTY process.
type Session struct {
	ID           string  `json:"id"`
	ProjectID    string  `json:"projectId"`
	Name         string  `json:"name"`         // e.g. branch name
	WorktreePath string  `json:"worktreePath"` // absolute path to git worktree
	ProcessID    *string `json:"processId"`    // nil when no PTY is running
	CreatedAt    int64   `json:"createdAt"`
	Status       string  `json:"status"` // "idle" | "active" | "stopped"
}

// Service is the workspace CRUD layer backed by bbolt.
type Service struct {
	db *bolt.DB
}

// New creates a Service and initialises the required bbolt buckets.
func New(db *bolt.DB) *Service {
	db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bucketProjects, bucketSessions, bucketSessionsByProject} {
			tx.CreateBucketIfNotExists(b)
		}
		return nil
	})
	return &Service{db: db}
}

// ── Projects ──────────────────────────────────────────────────────────────────

func (s *Service) CreateProject(_ context.Context, name, path string) (*Project, error) {
	p := &Project{
		ID:             uuid.New().String(),
		Name:           name,
		Path:           path,
		CreatedAt:      time.Now().Unix(),
		LastActivityAt: time.Now().Unix(),
	}
	return p, s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(p)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketProjects).Put([]byte(p.ID), data)
	})
}

func (s *Service) GetProject(_ context.Context, id string) (*Project, error) {
	var p Project
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketProjects).Get([]byte(id))
		if data == nil {
			return fmt.Errorf("project %s not found", id)
		}
		return json.Unmarshal(data, &p)
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Service) ListProjects(_ context.Context) ([]*Project, error) {
	var projects []*Project
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketProjects).ForEach(func(_, v []byte) error {
			var p Project
			if err := json.Unmarshal(v, &p); err != nil {
				return err
			}
			projects = append(projects, &p)
			return nil
		})
	})
	if projects == nil {
		projects = []*Project{}
	}
	return projects, err
}

func (s *Service) UpdateProject(_ context.Context, id, name string) (*Project, error) {
	p, err := s.GetProject(context.Background(), id)
	if err != nil {
		return nil, err
	}
	p.Name = name
	return p, s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(p)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketProjects).Put([]byte(p.ID), data)
	})
}

func (s *Service) DeleteProject(_ context.Context, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(bucketProjects).Delete([]byte(id)); err != nil {
			return err
		}
		// delete all session index entries for this project
		prefix := []byte(id + "/")
		idxBucket := tx.Bucket(bucketSessionsByProject)
		c := idxBucket.Cursor()
		for k, _ := c.Seek(prefix); k != nil && len(k) > len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = c.Next() {
			idxBucket.Delete(k)
		}
		return nil
	})
}

func (s *Service) TouchProject(_ context.Context, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketProjects).Get([]byte(id))
		if data == nil {
			return fmt.Errorf("project %s not found", id)
		}
		var p Project
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}
		p.LastActivityAt = time.Now().Unix()
		updated, err := json.Marshal(p)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketProjects).Put([]byte(id), updated)
	})
}

// ── Sessions ──────────────────────────────────────────────────────────────────

func (s *Service) CreateSession(_ context.Context, projectID, name, worktreePath string) (*Session, error) {
	sess := &Session{
		ID:           uuid.New().String(),
		ProjectID:    projectID,
		Name:         name,
		WorktreePath: worktreePath,
		ProcessID:    nil,
		CreatedAt:    time.Now().Unix(),
		Status:       "idle",
	}
	return sess, s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(sess)
		if err != nil {
			return err
		}
		if err := tx.Bucket(bucketSessions).Put([]byte(sess.ID), data); err != nil {
			return err
		}
		indexKey := []byte(projectID + "/" + sess.ID)
		return tx.Bucket(bucketSessionsByProject).Put(indexKey, []byte(""))
	})
}

func (s *Service) GetSession(_ context.Context, id string) (*Session, error) {
	var sess Session
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketSessions).Get([]byte(id))
		if data == nil {
			return fmt.Errorf("session %s not found", id)
		}
		return json.Unmarshal(data, &sess)
	})
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Service) ListSessions(_ context.Context, projectID string) ([]*Session, error) {
	var sessions []*Session
	err := s.db.View(func(tx *bolt.Tx) error {
		prefix := []byte(projectID + "/")
		c := tx.Bucket(bucketSessionsByProject).Cursor()
		for k, _ := c.Seek(prefix); k != nil && len(k) > len(prefix) && string(k[:len(prefix)]) == string(prefix); k, _ = c.Next() {
			sessionID := string(k[len(prefix):])
			data := tx.Bucket(bucketSessions).Get([]byte(sessionID))
			if data == nil {
				continue
			}
			var sess Session
			if err := json.Unmarshal(data, &sess); err != nil {
				return err
			}
			sessions = append(sessions, &sess)
		}
		return nil
	})
	if sessions == nil {
		sessions = []*Session{}
	}
	return sessions, err
}

func (s *Service) UpdateSessionStatus(_ context.Context, id, status string, processID *string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketSessions).Get([]byte(id))
		if data == nil {
			return fmt.Errorf("session %s not found", id)
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			return err
		}
		sess.Status = status
		sess.ProcessID = processID
		updated, err := json.Marshal(sess)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketSessions).Put([]byte(id), updated)
	})
}

func (s *Service) DeleteSession(_ context.Context, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketSessions).Get([]byte(id))
		if data == nil {
			return fmt.Errorf("session %s not found", id)
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			return err
		}
		if err := tx.Bucket(bucketSessions).Delete([]byte(id)); err != nil {
			return err
		}
		indexKey := []byte(sess.ProjectID + "/" + id)
		return tx.Bucket(bucketSessionsByProject).Delete(indexKey)
	})
}
