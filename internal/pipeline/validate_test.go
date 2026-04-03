package pipeline

import (
	"testing"

	"github.com/ian-howell/cicada/internal/model"
)

func TestValidate_MissingImage(t *testing.T) {
	p := &model.Pipeline{
		Name: "ci",
		Steps: []model.Step{
			{Name: "test", Commands: []string{"go test ./..."}},
		},
	}
	if err := Validate(p); err == nil {
		t.Error("Validate() expected error for missing image, got nil")
	}
}

func TestValidate_DuplicateStepName(t *testing.T) {
	p := &model.Pipeline{
		Name:  "ci",
		Image: "golang:1.22",
		Steps: []model.Step{
			{Name: "test", Image: "golang:1.22", Commands: []string{"go test ./..."}},
			{Name: "test", Image: "golang:1.22", Commands: []string{"go test ./..."}},
		},
	}
	if err := Validate(p); err == nil {
		t.Error("Validate() expected error for duplicate step name, got nil")
	}
}

func TestValidate_UnknownDependency(t *testing.T) {
	p := &model.Pipeline{
		Name:  "ci",
		Image: "golang:1.22",
		Steps: []model.Step{
			{Name: "test", Image: "golang:1.22", Commands: []string{"go test ./..."}, DependsOn: []string{"nonexistent"}},
		},
	}
	if err := Validate(p); err == nil {
		t.Error("Validate() expected error for unknown dependency, got nil")
	}
}

func TestValidate_CircularDependency(t *testing.T) {
	p := &model.Pipeline{
		Name:  "ci",
		Image: "golang:1.22",
		Steps: []model.Step{
			{Name: "a", Image: "golang:1.22", Commands: []string{"echo a"}, DependsOn: []string{"b"}},
			{Name: "b", Image: "golang:1.22", Commands: []string{"echo b"}, DependsOn: []string{"a"}},
		},
	}
	if err := Validate(p); err == nil {
		t.Error("Validate() expected error for circular dependency, got nil")
	}
}

func TestValidate_Valid(t *testing.T) {
	p := &model.Pipeline{
		Name:  "ci",
		Image: "golang:1.22",
		Steps: []model.Step{
			{Name: "vet", Image: "golang:1.22", Commands: []string{"go vet ./..."}},
			{Name: "test", Image: "golang:1.22", Commands: []string{"go test ./..."}, DependsOn: []string{"vet"}},
		},
	}
	if err := Validate(p); err != nil {
		t.Errorf("Validate() unexpected error = %v", err)
	}
}
