package store

import (
	"testing"
	"time"

	"github.com/ian-howell/cicada/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateAndGetBuild(t *testing.T) {
	s := newTestStore(t)

	b := &model.Build{
		ID:           "01HTEST00000000000000000001",
		PipelineName: "ci",
		Status:       model.StatusPending,
		Ref:          "refs/heads/main",
		CommitSHA:    "abc123",
		CloneURL:     "https://github.com/example/repo.git",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	}

	if err := s.CreateBuild(b); err != nil {
		t.Fatalf("CreateBuild() error = %v", err)
	}

	got, err := s.GetBuild(b.ID)
	if err != nil {
		t.Fatalf("GetBuild() error = %v", err)
	}

	if got.ID != b.ID {
		t.Errorf("ID = %q, want %q", got.ID, b.ID)
	}
	if got.Status != model.StatusPending {
		t.Errorf("Status = %q, want %q", got.Status, model.StatusPending)
	}
}

func TestUpdateBuildStatus(t *testing.T) {
	s := newTestStore(t)

	b := &model.Build{
		ID:           "01HTEST00000000000000000002",
		PipelineName: "ci",
		Status:       model.StatusPending,
		Ref:          "refs/heads/main",
		CommitSHA:    "abc123",
		CloneURL:     "https://github.com/example/repo.git",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	if err := s.CreateBuild(b); err != nil {
		t.Fatalf("CreateBuild() error = %v", err)
	}

	if err := s.UpdateBuildStatus(b.ID, model.StatusRunning, nil, nil); err != nil {
		t.Fatalf("UpdateBuildStatus() error = %v", err)
	}

	got, err := s.GetBuild(b.ID)
	if err != nil {
		t.Fatalf("GetBuild() error = %v", err)
	}
	if got.Status != model.StatusRunning {
		t.Errorf("Status = %q, want %q", got.Status, model.StatusRunning)
	}
}

func TestListBuilds(t *testing.T) {
	s := newTestStore(t)

	for _, id := range []string{
		"01HTEST00000000000000000010",
		"01HTEST00000000000000000011",
	} {
		b := &model.Build{
			ID:           id,
			PipelineName: "ci",
			Status:       model.StatusPending,
			Ref:          "refs/heads/main",
			CommitSHA:    "abc123",
			CloneURL:     "https://github.com/example/repo.git",
			CreatedAt:    time.Now().UTC().Truncate(time.Second),
		}
		if err := s.CreateBuild(b); err != nil {
			t.Fatalf("CreateBuild() error = %v", err)
		}
	}

	builds, err := s.ListBuilds()
	if err != nil {
		t.Fatalf("ListBuilds() error = %v", err)
	}
	if len(builds) != 2 {
		t.Errorf("len(builds) = %d, want 2", len(builds))
	}
}
