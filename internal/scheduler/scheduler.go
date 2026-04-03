package scheduler

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/ianhomer/cicada/internal/model"
	"github.com/ianhomer/cicada/internal/pipeline"
	"github.com/ianhomer/cicada/internal/store"
	"github.com/oklog/ulid/v2"
)

// RunFunc is a function that executes a build. The scheduler calls this in a goroutine.
type RunFunc func(build *model.Build)

// Scheduler receives ForgeEvents, creates Build records, and dispatches them to a runner.
type Scheduler struct {
	store *store.Store
	runFn RunFunc
}

// New creates a Scheduler with the given store and run function.
func New(s *store.Store, runFn RunFunc) *Scheduler {
	return &Scheduler{store: s, runFn: runFn}
}

// HandleEvent creates a build for the given event and pipeline name, then dispatches it asynchronously.
func (sc *Scheduler) HandleEvent(event *model.ForgeEvent, pipelineName string) error {
	build := &model.Build{
		ID:           ulid.Make().String(),
		PipelineName: pipelineName,
		Status:       model.StatusPending,
		Ref:          event.Ref,
		CommitSHA:    event.CommitSHA,
		CloneURL:     event.CloneURL,
		CreatedAt:    time.Now().UTC(),
	}

	if err := sc.store.CreateBuild(build); err != nil {
		return fmt.Errorf("create build: %w", err)
	}

	log.Printf("scheduler: dispatching build %s (pipeline=%s ref=%s)", build.ID, pipelineName, event.Ref)
	go sc.runFn(build)

	return nil
}

// DispatchForgeEvent reads pipeline definitions from cloneDir/.cicada, filters by
// event trigger type, and dispatches one build per matching pipeline to the runner.
func (sc *Scheduler) DispatchForgeEvent(event *model.ForgeEvent, cloneDir string) error {
	pipelines, err := pipeline.ParseDir(filepath.Join(cloneDir, ".cicada"))
	if err != nil {
		return fmt.Errorf("parse pipelines: %w", err)
	}

	for _, p := range pipelines {
		triggered := false
		for _, trigger := range p.Triggers {
			if trigger == event.Type {
				triggered = true
				break
			}
		}
		if !triggered {
			continue
		}

		build := &model.Build{
			ID:           ulid.Make().String(),
			PipelineName: p.Name,
			Status:       model.StatusPending,
			Ref:          event.Ref,
			CommitSHA:    event.CommitSHA,
			CloneURL:     event.CloneURL,
			CreatedAt:    time.Now().UTC(),
		}

		if err := sc.store.CreateBuild(build); err != nil {
			log.Printf("scheduler: failed to create build for pipeline %q: %v", p.Name, err)
			continue
		}

		log.Printf("scheduler: dispatching build %s (pipeline=%s)", build.ID, p.Name)
		go sc.runFn(build)
	}
	return nil
}
