package pipeline

import (
	"github.com/ianhomer/cicada/internal/model"
)

// TopologicalOrder returns steps in an order where each step appears after
// all of its dependencies. Assumes the pipeline has already been validated (no cycles).
func TopologicalOrder(steps []model.Step) []model.Step {
	deps := make(map[string][]string, len(steps))
	for _, s := range steps {
		deps[s.Name] = s.DependsOn
	}

	visited := make(map[string]bool, len(steps))
	var result []model.Step

	// Index steps by name for lookup.
	byName := make(map[string]model.Step, len(steps))
	for _, s := range steps {
		byName[s.Name] = s
	}

	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		for _, dep := range deps[name] {
			visit(dep)
		}
		visited[name] = true
		result = append(result, byName[name])
	}

	for _, s := range steps {
		visit(s.Name)
	}
	return result
}
