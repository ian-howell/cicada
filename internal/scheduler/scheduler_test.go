package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ian-howell/cicada/internal/model"
	"github.com/ian-howell/cicada/internal/store"
)

func TestScheduler_HandleEvent_CreatesBuild(t *testing.T) {
	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	defer s.Close()

	ranCh := make(chan string, 1)
	fakeRun := func(build *model.Build) {
		ranCh <- build.ID
	}

	sched := New(s, fakeRun)

	event := &model.ForgeEvent{
		Type:      model.EventPush,
		Repo:      "example/repo",
		CloneURL:  "https://github.com/example/repo.git",
		Ref:       "refs/heads/main",
		CommitSHA: "abc123",
		Sender:    "octocat",
	}

	if err := sched.HandleEvent(event, "ci"); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	select {
	case ranBuildID := <-ranCh:
		if ranBuildID == "" {
			t.Error("expected non-empty build ID from runner")
		}
	case <-time.After(time.Second):
		t.Error("expected runner to be called within 1 second, but it was not")
	}

	builds, err := s.ListBuilds()
	if err != nil {
		t.Fatalf("ListBuilds() error = %v", err)
	}
	if len(builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(builds))
	}
	if builds[0].CommitSHA != "abc123" {
		t.Errorf("CommitSHA = %q, want %q", builds[0].CommitSHA, "abc123")
	}
}

func TestScheduler_DispatchForgeEvent_MatchingTrigger(t *testing.T) {
	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	defer s.Close()

	ranCh := make(chan string, 1)
	fakeRun := func(build *model.Build) {
		ranCh <- build.ID
	}
	sched := New(s, fakeRun)

	cloneDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cloneDir, ".cicada"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	pipelineYAML := `
name: ci
image: golang:1.22
on:
  - push
steps:
  - name: test
    commands:
      - go test ./...
`
	if err := os.WriteFile(filepath.Join(cloneDir, ".cicada", "ci.yml"), []byte(pipelineYAML), 0644); err != nil {
		t.Fatalf("write pipeline: %v", err)
	}

	event := &model.ForgeEvent{
		Type:      model.EventPush,
		Repo:      "example/repo",
		CloneURL:  "https://github.com/example/repo.git",
		Ref:       "refs/heads/main",
		CommitSHA: "abc123",
		Sender:    "octocat",
	}

	if err := sched.DispatchForgeEvent(event, cloneDir); err != nil {
		t.Fatalf("DispatchForgeEvent() error = %v", err)
	}

	select {
	case id := <-ranCh:
		if id == "" {
			t.Error("expected non-empty build ID")
		}
	case <-time.After(time.Second):
		t.Error("expected runner to be called within 1 second")
	}

	builds, err := s.ListBuilds()
	if err != nil {
		t.Fatalf("ListBuilds() error = %v", err)
	}
	if len(builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(builds))
	}
}

func TestScheduler_DispatchForgeEvent_NonMatchingTrigger(t *testing.T) {
	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	defer s.Close()

	ranCh := make(chan string, 1)
	fakeRun := func(build *model.Build) {
		ranCh <- build.ID
	}
	sched := New(s, fakeRun)

	cloneDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cloneDir, ".cicada"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Pipeline only triggers on pull_request, not push
	pipelineYAML := `
name: pr-only
image: golang:1.22
on:
  - pull_request
steps:
  - name: test
    commands:
      - go test ./...
`
	if err := os.WriteFile(filepath.Join(cloneDir, ".cicada", "pr-only.yml"), []byte(pipelineYAML), 0644); err != nil {
		t.Fatalf("write pipeline: %v", err)
	}

	event := &model.ForgeEvent{
		Type:      model.EventPush, // push, but pipeline only triggers on pull_request
		CloneURL:  "https://github.com/example/repo.git",
		Ref:       "refs/heads/main",
		CommitSHA: "abc123",
	}

	if err := sched.DispatchForgeEvent(event, cloneDir); err != nil {
		t.Fatalf("DispatchForgeEvent() error = %v", err)
	}

	select {
	case <-ranCh:
		t.Error("runner should NOT have been called for non-matching trigger")
	case <-time.After(100 * time.Millisecond):
		// correct — runner was not called
	}

	builds, _ := s.ListBuilds()
	if len(builds) != 0 {
		t.Errorf("expected 0 builds, got %d", len(builds))
	}
}
