package runner

import (
	"context"
	"os"
	"testing"

	"github.com/ian-howell/cicada/internal/model"
	"github.com/ian-howell/cicada/internal/store"
)

func TestDAGExecutor_SerialSteps(t *testing.T) {
	if os.Getenv("CICADA_TEST_DOCKER") == "" {
		t.Skip("set CICADA_TEST_DOCKER=1 to run Docker integration tests")
	}

	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	defer s.Close()

	build := &model.Build{
		ID:           "01HTEST00000000000000000099",
		PipelineName: "ci",
		Status:       model.StatusRunning,
	}

	// Insert the build record so step_results FK constraint is satisfied.
	if err := s.CreateBuild(build); err != nil {
		t.Fatalf("CreateBuild() error = %v", err)
	}

	pipeline := &model.Pipeline{
		Name: "ci",
		Steps: []model.Step{
			{Name: "step1", Image: "alpine:latest", Commands: []string{"echo step1"}},
			{Name: "step2", Image: "alpine:latest", Commands: []string{"echo step2"}, DependsOn: []string{"step1"}},
		},
	}

	r := New(s, t.TempDir())
	ctx := context.Background()

	volumeName, err := CreateWorkspaceVolume(ctx, "cicada-dag-test-"+t.Name())
	if err != nil {
		t.Fatalf("CreateWorkspaceVolume() error = %v", err)
	}
	defer RemoveWorkspaceVolume(ctx, volumeName)

	if err := r.executeDAG(ctx, build, pipeline, volumeName, t.TempDir()); err != nil {
		t.Fatalf("executeDAG() error = %v", err)
	}

	results, err := s.ListStepResults(build.ID)
	if err != nil {
		t.Fatalf("ListStepResults() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 step results, got %d", len(results))
	}
	for _, sr := range results {
		if sr.Status != model.StatusSuccess {
			t.Errorf("step %q status = %q, want success", sr.StepName, sr.Status)
		}
	}
}
