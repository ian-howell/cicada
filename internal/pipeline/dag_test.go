package pipeline

import (
	"testing"

	"github.com/ianhomer/cicada/internal/model"
)

func TestTopologicalOrder_Simple(t *testing.T) {
	steps := []model.Step{
		{Name: "test", Commands: []string{"go test ./..."}, DependsOn: []string{"vet"}},
		{Name: "vet", Commands: []string{"go vet ./..."}},
	}

	ordered := TopologicalOrder(steps)
	if len(ordered) != 2 {
		t.Fatalf("len = %d, want 2", len(ordered))
	}
	if ordered[0].Name != "vet" {
		t.Errorf("ordered[0] = %q, want %q (dependency must come first)", ordered[0].Name, "vet")
	}
	if ordered[1].Name != "test" {
		t.Errorf("ordered[1] = %q, want %q", ordered[1].Name, "test")
	}
}

func TestTopologicalOrder_NoDepStepsAreIndependent(t *testing.T) {
	steps := []model.Step{
		{Name: "lint", Commands: []string{"golangci-lint run"}},
		{Name: "vet", Commands: []string{"go vet ./..."}},
	}

	ordered := TopologicalOrder(steps)
	if len(ordered) != 2 {
		t.Fatalf("len = %d, want 2", len(ordered))
	}
}
