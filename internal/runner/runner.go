package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ianhomer/cicada/internal/model"
	"github.com/ianhomer/cicada/internal/pipeline"
	"github.com/ianhomer/cicada/internal/store"
	"github.com/oklog/ulid/v2"
)

// Runner executes builds by cloning repos, parsing pipelines, and running Docker containers.
type Runner struct {
	store   *store.Store
	dataDir string
}

// New creates a Runner.
func New(s *store.Store, dataDir string) *Runner {
	return &Runner{store: s, dataDir: dataDir}
}

// Run executes a build: clone, parse, execute DAG, update store.
// This is the top-level orchestration function.
func (r *Runner) Run(ctx context.Context, build *model.Build) error {
	log.Printf("runner: starting build %s (pipeline=%s ref=%s)", build.ID, build.PipelineName, build.Ref)

	now := time.Now().UTC()
	if err := r.store.UpdateBuildStatus(build.ID, model.StatusRunning, &now, nil); err != nil {
		return fmt.Errorf("update build status running: %w", err)
	}

	finalStatus := model.StatusSuccess
	if err := r.runBuild(ctx, build); err != nil {
		log.Printf("runner: build %s failed: %v", build.ID, err)
		finalStatus = model.StatusFailure
	}

	fin := time.Now().UTC()
	if err := r.store.UpdateBuildStatus(build.ID, finalStatus, nil, &fin); err != nil {
		log.Printf("runner: failed to update final build status: %v", err)
	}
	log.Printf("runner: build %s finished with status %s", build.ID, finalStatus)
	return nil
}

func (r *Runner) runBuild(ctx context.Context, build *model.Build) error {
	// 1. Clone repo.
	repoDir, err := os.MkdirTemp("", "cicada-clone-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(repoDir)

	if err := CloneRepo(ctx, build.CloneURL, build.CommitSHA, repoDir); err != nil {
		return fmt.Errorf("clone repo: %w", err)
	}

	// 2. Parse pipelines.
	pipelineDir := filepath.Join(repoDir, ".cicada")
	pipelines, err := pipeline.ParseDir(pipelineDir)
	if err != nil {
		return fmt.Errorf("parse pipelines: %w", err)
	}

	// Find the matching pipeline.
	var p *model.Pipeline
	for _, pl := range pipelines {
		if pl.Name == build.PipelineName {
			p = pl
			break
		}
	}
	if p == nil {
		return fmt.Errorf("pipeline %q not found in repo", build.PipelineName)
	}

	// 3. Create workspace volume.
	volumeName := "cicada-ws-" + build.ID
	if _, err := CreateWorkspaceVolume(ctx, volumeName); err != nil {
		return fmt.Errorf("create workspace volume: %w", err)
	}
	defer func() {
		if err := RemoveWorkspaceVolume(ctx, volumeName); err != nil {
			log.Printf("runner: failed to remove workspace volume %s: %v", volumeName, err)
		}
	}()

	// 4. Execute DAG.
	return r.executeDAG(ctx, build, p, volumeName, repoDir)
}

// newBuildID generates a new ULID for use as a build ID.
// Called by the scheduler when creating new Build records.
func newBuildID() string {
	return ulid.Make().String()
}

// executeDAG is implemented in dag_executor.go (Task 8).
// Stub here to allow compilation.
func (r *Runner) executeDAG(ctx context.Context, build *model.Build, p *model.Pipeline, volumeName, repoDir string) error {
	return fmt.Errorf("executeDAG not yet implemented")
}
