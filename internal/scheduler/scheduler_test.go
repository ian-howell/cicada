package scheduler

import (
	"testing"
	"time"

	"github.com/ianhomer/cicada/internal/model"
	"github.com/ianhomer/cicada/internal/store"
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
