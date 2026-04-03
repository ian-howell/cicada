package runner

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/ianhomer/cicada/internal/model"
	"github.com/ianhomer/cicada/internal/pipeline"
)

// executeDAG runs pipeline steps concurrently according to their dependency graph.
// Failed steps cause dependents to be marked cancelled.
func (r *Runner) executeDAG(ctx context.Context, build *model.Build, p *model.Pipeline, volumeName, repoDir string) error {
	steps := pipeline.TopologicalOrder(p.Steps)

	logDir := filepath.Join(r.dataDir, "logs", build.ID)

	// Initialize step results in store.
	for _, step := range steps {
		logFile := filepath.Join("logs", build.ID, step.Name+".log")
		sr := &model.StepResult{
			BuildID:  build.ID,
			StepName: step.Name,
			Status:   model.StatusPending,
			LogFile:  logFile,
		}
		if err := r.store.CreateStepResult(sr); err != nil {
			return fmt.Errorf("create step result for %q: %w", step.Name, err)
		}
	}

	// Track completion status of each step.
	stepStatus := make(map[string]model.BuildStatus, len(steps))
	statusMu := sync.Mutex{}
	sem := make(chan struct{}, runtime.NumCPU())

	var wg sync.WaitGroup
	// stepDone signals when a step's status is finalized.
	stepDone := make(map[string]chan struct{}, len(steps))
	for _, step := range steps {
		stepDone[step.Name] = make(chan struct{})
	}

	for _, step := range steps {
		step := step // capture loop variable
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(stepDone[step.Name])

			// Wait for all dependencies.
			cancelled := false
			for _, dep := range step.DependsOn {
				select {
				case <-stepDone[dep]:
					statusMu.Lock()
					depStatus := stepStatus[dep]
					statusMu.Unlock()
					if depStatus != model.StatusSuccess {
						cancelled = true
					}
				case <-ctx.Done():
					statusMu.Lock()
					stepStatus[step.Name] = model.StatusCancelled
					statusMu.Unlock()
					if err := r.store.UpdateStepResult(build.ID, step.Name, model.StatusCancelled, 0, nil, nil); err != nil {
						log.Printf("runner: failed to update step result for %q: %v", step.Name, err)
					}
					return
				}
			}

			if cancelled {
				log.Printf("runner: step %q cancelled (dependency failed)", step.Name)
				statusMu.Lock()
				stepStatus[step.Name] = model.StatusCancelled
				statusMu.Unlock()
				if err := r.store.UpdateStepResult(build.ID, step.Name, model.StatusCancelled, 0, nil, nil); err != nil {
					log.Printf("runner: failed to update step result for %q: %v", step.Name, err)
				}
				return
			}

			// Acquire concurrency semaphore.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				statusMu.Lock()
				stepStatus[step.Name] = model.StatusCancelled
				statusMu.Unlock()
				if err := r.store.UpdateStepResult(build.ID, step.Name, model.StatusCancelled, 0, nil, nil); err != nil {
					log.Printf("runner: failed to update step result for %q: %v", step.Name, err)
				}
				return
			}
			defer func() { <-sem }()

			now := time.Now().UTC()
			if err := r.store.UpdateStepResult(build.ID, step.Name, model.StatusRunning, 0, &now, nil); err != nil {
				log.Printf("runner: failed to update step result for %q: %v", step.Name, err)
			}

			log.Printf("runner: executing step %q", step.Name)
			exitCode, err := RunStep(ctx, step, volumeName, repoDir, logDir)
			fin := time.Now().UTC()

			if err != nil {
				log.Printf("runner: step %q system error: %v", step.Name, err)
				statusMu.Lock()
				stepStatus[step.Name] = model.StatusFailure
				statusMu.Unlock()
				if err := r.store.UpdateStepResult(build.ID, step.Name, model.StatusFailure, -1, &now, &fin); err != nil {
					log.Printf("runner: failed to update step result for %q: %v", step.Name, err)
				}
				return
			}

			status := model.StatusSuccess
			if exitCode != 0 {
				status = model.StatusFailure
			}
			statusMu.Lock()
			stepStatus[step.Name] = status
			statusMu.Unlock()
			if err := r.store.UpdateStepResult(build.ID, step.Name, status, exitCode, &now, &fin); err != nil {
				log.Printf("runner: failed to update step result for %q: %v", step.Name, err)
			}
			log.Printf("runner: step %q finished with status %s (exit=%d)", step.Name, status, exitCode)
		}()
	}

	wg.Wait()

	// Determine overall result.
	for _, step := range steps {
		if stepStatus[step.Name] == model.StatusFailure {
			return fmt.Errorf("step %q failed", step.Name)
		}
	}
	return nil
}
