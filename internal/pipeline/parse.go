package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ian-howell/cicada/internal/model"
	"gopkg.in/yaml.v3"
)

// rawPipeline is the YAML structure before normalization.
type rawPipeline struct {
	Name  string    `yaml:"name"`
	Image string    `yaml:"image"`
	On    []string  `yaml:"on"`
	Steps []rawStep `yaml:"steps"`
}

type rawStep struct {
	Name      string            `yaml:"name"`
	Image     string            `yaml:"image"`
	Commands  []string          `yaml:"commands"`
	Env       map[string]string `yaml:"env"`
	DependsOn []string          `yaml:"dependsOn"`
}

// ParseFile reads and parses a .cicada/*.yml pipeline file.
// Images are resolved at parse time: step images fall back to the pipeline image.
// The pipeline name defaults to the filename stem if not specified.
func ParseFile(path string) (*model.Pipeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pipeline file: %w", err)
	}

	var raw rawPipeline
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse pipeline YAML: %w", err)
	}

	name := raw.Name
	if name == "" {
		base := filepath.Base(path)
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}

	var triggers []model.EventType
	for _, t := range raw.On {
		triggers = append(triggers, model.EventType(t))
	}

	steps := make([]model.Step, len(raw.Steps))
	for i, rs := range raw.Steps {
		img := rs.Image
		if img == "" {
			img = raw.Image
		}
		steps[i] = model.Step{
			Name:      rs.Name,
			Image:     img,
			Commands:  rs.Commands,
			Env:       rs.Env,
			DependsOn: rs.DependsOn,
		}
	}

	return &model.Pipeline{
		Name:     name,
		Image:    raw.Image,
		Triggers: triggers,
		Steps:    steps,
	}, nil
}

// ParseDir reads all *.yml and *.yaml files from a directory and returns all valid pipelines.
func ParseDir(dir string) ([]*model.Pipeline, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read pipeline dir: %w", err)
	}

	var pipelines []*model.Pipeline
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yml") && !strings.HasSuffix(entry.Name(), ".yaml")) {
			continue
		}
		p, err := ParseFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", entry.Name(), err)
		}
		pipelines = append(pipelines, p)
	}
	return pipelines, nil
}
