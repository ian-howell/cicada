package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ian-howell/cicada/internal/model"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "ci.yml", `
name: ci
image: golang:1.22
on:
  - push
steps:
  - name: vet
    commands:
      - go vet ./...
  - name: test
    commands:
      - go test ./...
    dependsOn:
      - vet
`)

	p, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if p.Name != "ci" {
		t.Errorf("Name = %q, want %q", p.Name, "ci")
	}
	if len(p.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(p.Steps))
	}
	if p.Steps[0].Image != "golang:1.22" {
		t.Errorf("Steps[0].Image = %q, want %q", p.Steps[0].Image, "golang:1.22")
	}
	if p.Steps[1].Image != "golang:1.22" {
		t.Errorf("Steps[1].Image = %q, want %q (should inherit pipeline image)", p.Steps[1].Image, "golang:1.22")
	}
	if len(p.Steps[1].DependsOn) != 1 || p.Steps[1].DependsOn[0] != "vet" {
		t.Errorf("Steps[1].DependsOn = %v, want [vet]", p.Steps[1].DependsOn)
	}
}

func TestParseFile_StepImageOverride(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "ci.yml", `
name: ci
image: golang:1.22
on:
  - push
steps:
  - name: build
    image: golang:1.22-alpine
    commands:
      - go build ./...
`)
	p, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if p.Steps[0].Image != "golang:1.22-alpine" {
		t.Errorf("Steps[0].Image = %q, want %q", p.Steps[0].Image, "golang:1.22-alpine")
	}
}

func TestParseFile_Triggers(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "ci.yml", `
name: ci
image: golang:1.22
on:
  - push
  - pull_request
  - tag
steps:
  - name: build
    commands:
      - go build ./...
`)
	p, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if len(p.Triggers) != 3 {
		t.Errorf("len(Triggers) = %d, want 3", len(p.Triggers))
	}
}

func TestParseFile_DefaultName(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "mypipeline.yml", `
image: golang:1.22
steps:
  - name: build
    commands:
      - go build ./...
`)
	p, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if p.Name != "mypipeline" {
		t.Errorf("Name = %q, want %q", p.Name, "mypipeline")
	}
}

func TestParseDir(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "alpha.yml", `
name: alpha
image: golang:1.22
steps:
  - name: build
    commands:
      - go build ./...
`)
	writeFile(t, dir, "beta.yaml", `
name: beta
image: golang:1.22
steps:
  - name: test
    commands:
      - go test ./...
`)
	writeFile(t, dir, "notes.txt", "this file should be ignored")

	pipelines, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir() error = %v", err)
	}
	if len(pipelines) != 2 {
		t.Fatalf("len(pipelines) = %d, want 2", len(pipelines))
	}

	names := make(map[string]bool, len(pipelines))
	for _, p := range pipelines {
		names[p.Name] = true
	}
	if !names["alpha"] {
		t.Errorf("expected pipeline %q to be present", "alpha")
	}
	if !names["beta"] {
		t.Errorf("expected pipeline %q to be present", "beta")
	}
}

// Compile-time check that the model import is used.
var _ model.Pipeline
