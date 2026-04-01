package workspaces_test

import (
	"context"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"

	"github.com/ms/amplifier-app-loom/internal/workspaces"
)

func openTestDB(t *testing.T) *bolt.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateProject(t *testing.T) {
	svc := workspaces.New(openTestDB(t))
	ctx := context.Background()

	p, err := svc.CreateProject(ctx, "loom", "/tmp/loom")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if p.Name != "loom" {
		t.Fatalf("expected name loom, got %s", p.Name)
	}
	if p.Path != "/tmp/loom" {
		t.Fatalf("expected path /tmp/loom, got %s", p.Path)
	}
}

func TestListProjects(t *testing.T) {
	svc := workspaces.New(openTestDB(t))
	ctx := context.Background()

	svc.CreateProject(ctx, "alpha", "/tmp/alpha")
	svc.CreateProject(ctx, "beta", "/tmp/beta")

	projects, err := svc.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
}

func TestDeleteProject(t *testing.T) {
	svc := workspaces.New(openTestDB(t))
	ctx := context.Background()

	p, _ := svc.CreateProject(ctx, "toDelete", "/tmp/del")
	if err := svc.DeleteProject(ctx, p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	got, err := svc.GetProject(ctx, p.ID)
	if err == nil && got != nil {
		t.Fatal("expected project to be deleted")
	}
}

func TestCreateSession(t *testing.T) {
	svc := workspaces.New(openTestDB(t))
	ctx := context.Background()

	dir := t.TempDir()
	p, _ := svc.CreateProject(ctx, "proj", dir)

	s, err := svc.CreateSession(ctx, p.ID, "main", dir)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if s.ProjectID != p.ID {
		t.Fatalf("expected projectID %s, got %s", p.ID, s.ProjectID)
	}
	if s.Status != "idle" {
		t.Fatalf("expected status idle, got %s", s.Status)
	}
}

func TestListSessionsForProject(t *testing.T) {
	svc := workspaces.New(openTestDB(t))
	ctx := context.Background()

	dir := t.TempDir()
	p, _ := svc.CreateProject(ctx, "proj", dir)
	svc.CreateSession(ctx, p.ID, "main", dir)
	svc.CreateSession(ctx, p.ID, "feature", filepath.Join(dir, "feature"))

	sessions, err := svc.ListSessions(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}
