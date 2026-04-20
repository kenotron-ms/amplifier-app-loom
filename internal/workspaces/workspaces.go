package workspaces

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

var bucketProjects = []byte("projects")

// Project is a codebase on disk managed by Loom.
type Project struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Path           string `json:"path"`      // absolute path on disk
	Workspace      string `json:"workspace"` // grouping label (default: "Default")
	CreatedAt      int64  `json:"createdAt"`
	LastActivityAt int64  `json:"lastActivityAt"`
}

// Service is the workspace CRUD layer backed by bbolt.
type Service struct {
	db *bolt.DB
}

// New creates a Service and initialises the required bbolt buckets.
func New(db *bolt.DB) (*Service, error) {
	err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketProjects); err != nil {
			return fmt.Errorf("create bucket %q: %w", bucketProjects, err)
		}
		if _, err := tx.CreateBucketIfNotExists(bucketSessions); err != nil {
			return fmt.Errorf("create bucket %q: %w", bucketSessions, err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("initialise workspaces buckets: %w", err)
	}
	return &Service{db: db}, nil
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
	if p.Workspace == "" {
		p.Workspace = "Default"
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
			if p.Workspace == "" {
				p.Workspace = "Default"
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

func (s *Service) UpdateProject(_ context.Context, id, name, workspace string) (*Project, error) {
	var p Project
	return &p, s.db.Update(func(tx *bolt.Tx) error {
		data := tx.Bucket(bucketProjects).Get([]byte(id))
		if data == nil {
			return fmt.Errorf("project %s not found", id)
		}
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}
		p.Name = name
		if workspace != "" {
			p.Workspace = workspace
		}
		if p.Workspace == "" {
			p.Workspace = "Default"
		}
		updated, err := json.Marshal(p)
		if err != nil {
			return err
		}
		return tx.Bucket(bucketProjects).Put([]byte(id), updated)
	})
}

func (s *Service) DeleteProject(_ context.Context, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if b := tx.Bucket(bucketProjects); b != nil {
			if err := b.Delete([]byte(id)); err != nil {
				return err
			}
		}
		return s.deleteSessionsForProject(tx, id)
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

    var bucketSessions = []byte("sessions")

    // Session is a working context (git worktree / branch) within a Project.
    type Session struct {
    	ID        string `json:"id"`
    	ProjectID string `json:"projectID"`
    	Branch    string `json:"branch"`
    	Workdir   string `json:"workdir"`
    	Status    string `json:"status"`
    	CreatedAt int64  `json:"createdAt"`
    }

    func (s *Service) CreateSession(_ context.Context, projectID, branch, workdir string) (*Session, error) {
    	sess := &Session{
    		ID:        uuid.New().String(),
    		ProjectID: projectID,
    		Branch:    branch,
    		Workdir:   workdir,
    		Status:    "idle",
    		CreatedAt: time.Now().Unix(),
    	}
    	return sess, s.db.Update(func(tx *bolt.Tx) error {
    		b, err := tx.CreateBucketIfNotExists(bucketSessions)
    		if err != nil {
    			return err
    		}
    		data, err := json.Marshal(sess)
    		if err != nil {
    			return err
    		}
    		return b.Put([]byte(sess.ID), data)
    	})
    }

    func (s *Service) ListSessions(_ context.Context, projectID string) ([]*Session, error) {
    	var out []*Session
    	return out, s.db.View(func(tx *bolt.Tx) error {
    		b := tx.Bucket(bucketSessions)
    		if b == nil {
    			return nil
    		}
    		return b.ForEach(func(_, v []byte) error {
    			var sess Session
    			if err := json.Unmarshal(v, &sess); err != nil {
    				return err
    			}
    			if sess.ProjectID == projectID {
    				cp := sess
    				out = append(out, &cp)
    			}
    			return nil
    		})
    	})
    }

    func (s *Service) GetSession(_ context.Context, id string) (*Session, error) {
    	var sess Session
    	err := s.db.View(func(tx *bolt.Tx) error {
    		b := tx.Bucket(bucketSessions)
    		if b == nil {
    			return fmt.Errorf("session %q not found", id)
    		}
    		v := b.Get([]byte(id))
    		if v == nil {
    			return fmt.Errorf("session %q not found", id)
    		}
    		return json.Unmarshal(v, &sess)
    	})
    	if err != nil {
    		return nil, err
    	}
    	return &sess, nil
    }

    func (s *Service) deleteSessionsForProject(tx *bolt.Tx, projectID string) error {
    	b := tx.Bucket(bucketSessions)
    	if b == nil {
    		return nil
    	}
    	var toDelete [][]byte
    	_ = b.ForEach(func(k, v []byte) error {
    		var sess Session
    		if err := json.Unmarshal(v, &sess); err == nil && sess.ProjectID == projectID {
    			cp := make([]byte, len(k))
    			copy(cp, k)
    			toDelete = append(toDelete, cp)
    		}
    		return nil
    	})
    	for _, k := range toDelete {
    		if err := b.Delete(k); err != nil {
    			return err
    		}
    	}
    	return nil
    }
    