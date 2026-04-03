package pipeline

import (
	"fmt"

	"github.com/ian-howell/cicada/internal/model"
)

// Validate checks that a pipeline is structurally valid:
// all steps have images, names are unique, dependencies exist, no cycles.
func Validate(p *model.Pipeline) error {
	names := make(map[string]bool, len(p.Steps))
	for _, s := range p.Steps {
		if s.Image == "" {
			return fmt.Errorf("step %q has no image (set pipeline-level image or step-level image)", s.Name)
		}
		if names[s.Name] {
			return fmt.Errorf("duplicate step name %q", s.Name)
		}
		names[s.Name] = true
	}

	for _, s := range p.Steps {
		for _, dep := range s.DependsOn {
			if !names[dep] {
				return fmt.Errorf("step %q depends on unknown step %q", s.Name, dep)
			}
		}
	}

	if err := checkCycles(p.Steps); err != nil {
		return err
	}

	return nil
}

func checkCycles(steps []model.Step) error {
	// Build adjacency map.
	deps := make(map[string][]string, len(steps))
	for _, s := range steps {
		deps[s.Name] = s.DependsOn
	}

	// DFS-based cycle detection.
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	state := make(map[string]int, len(steps))

	var visit func(name string) error
	visit = func(name string) error {
		if state[name] == visited {
			return nil
		}
		if state[name] == visiting {
			return fmt.Errorf("circular dependency detected involving step %q", name)
		}
		state[name] = visiting
		for _, dep := range deps[name] {
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[name] = visited
		return nil
	}

	for _, s := range steps {
		if err := visit(s.Name); err != nil {
			return err
		}
	}
	return nil
}
