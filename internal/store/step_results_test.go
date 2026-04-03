package store

import (
	"testing"
	"time"

	"github.com/ianhomer/cicada/internal/model"
)

func TestCreateAndListStepResults(t *testing.T) {
	s := newTestStore(t)

	build := &model.Build{
		ID:           "01HTEST00000000000000000020",
		PipelineName: "ci",
		Status:       model.StatusRunning,
		Ref:          "refs/heads/main",
		CommitSHA:    "abc123",
		CloneURL:     "https://github.com/example/repo.git",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	if err := s.CreateBuild(build); err != nil {
		t.Fatalf("CreateBuild() error = %v", err)
	}

	sr := &model.StepResult{
		BuildID:  build.ID,
		StepName: "test",
		Status:   model.StatusPending,
		LogFile:  "logs/01HTEST00000000000000000020/test.log",
	}
	if err := s.CreateStepResult(sr); err != nil {
		t.Fatalf("CreateStepResult() error = %v", err)
	}

	results, err := s.ListStepResults(build.ID)
	if err != nil {
		t.Fatalf("ListStepResults() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].StepName != "test" {
		t.Errorf("StepName = %q, want %q", results[0].StepName, "test")
	}
}

func TestUpdateStepResult(t *testing.T) {
	s := newTestStore(t)

	build := &model.Build{
		ID:           "01HTEST00000000000000000030",
		PipelineName: "ci",
		Status:       model.StatusRunning,
		Ref:          "refs/heads/main",
		CommitSHA:    "abc123",
		CloneURL:     "https://github.com/example/repo.git",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	if err := s.CreateBuild(build); err != nil {
		t.Fatalf("CreateBuild() error = %v", err)
	}

	sr := &model.StepResult{
		BuildID:  build.ID,
		StepName: "vet",
		Status:   model.StatusRunning,
		LogFile:  "logs/01HTEST00000000000000000030/vet.log",
	}
	if err := s.CreateStepResult(sr); err != nil {
		t.Fatalf("CreateStepResult() error = %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := s.UpdateStepResult(build.ID, "vet", model.StatusSuccess, 0, nil, &now); err != nil {
		t.Fatalf("UpdateStepResult() error = %v", err)
	}

	results, err := s.ListStepResults(build.ID)
	if err != nil {
		t.Fatalf("ListStepResults() error = %v", err)
	}
	if results[0].Status != model.StatusSuccess {
		t.Errorf("Status = %q, want %q", results[0].Status, model.StatusSuccess)
	}
}
