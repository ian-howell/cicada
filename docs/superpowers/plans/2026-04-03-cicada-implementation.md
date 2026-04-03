# Cicada Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Cicada, a self-hosted CI/CD platform that receives GitHub webhooks, clones repos, parses `.cicada/*.yml` pipeline definitions, executes build steps in Docker containers with DAG-based concurrency, and provides a web UI with live log streaming.

**Architecture:** Single Go binary; flat monolithic package structure under `internal/`; direct function calls and Go channels for internal communication. SQLite for persistence, Docker SDK for container execution, HTMX + SSE for the web UI.

**Tech Stack:** Go 1.22+, `modernc.org/sqlite` (pure Go SQLite driver), `github.com/docker/docker/client` (Docker SDK), `github.com/oklog/ulid/v2` (ULIDs), `gopkg.in/yaml.v3` (YAML parsing), Pico.css + HTMX (embedded static assets).

---

## File Map

```
cmd/cicada/
  main.go                         # entrypoint: flags, wiring, server start

internal/model/
  model.go                        # all domain types: Pipeline, Step, Build, StepResult, BuildStatus, EventType, ForgeEvent

internal/store/
  store.go                        # Store struct + New(), Close(), migration runner
  builds.go                       # CreateBuild, GetBuild, ListBuilds, UpdateBuildStatus
  step_results.go                 # CreateStepResult, UpdateStepResult, ListStepResults
  migrations/
    001_initial.sql               # CREATE TABLE builds, step_results

internal/pipeline/
  parse.go                        # ParseFile(path) (*model.Pipeline, error)
  validate.go                     # Validate(*model.Pipeline) error — image resolution, unique names, DAG cycle check
  dag.go                          # BuildDAG, TopologicalSort, dependency resolution helpers

internal/runner/
  runner.go                       # Runner struct, Run(ctx, build, pipelines) — orchestrates full build lifecycle
  clone.go                        # CloneRepo(ctx, cloneURL, commitSHA, destDir) error
  dag_executor.go                 # executes steps concurrently per DAG topology
  step_executor.go                # runs a single step in a Docker container, streams logs
  workspace.go                    # create/remove Docker volume for workspace

internal/webhook/
  provider.go                     # ForgeProvider interface
  registry.go                     # Registry: map[string]ForgeProvider, Register, Get
  github.go                       # GitHubProvider: ParseWebhook, signature validation

internal/scheduler/
  scheduler.go                    # Scheduler: receives ForgeEvents, creates Builds, dispatches to Runner

internal/web/
  server.go                       # Server struct, route registration, Start()
  handlers_builds.go              # GET /builds, GET /builds/{id}
  handlers_webhook.go             # POST /webhooks/{provider}
  handlers_log.go                 # GET /builds/{id}/steps/{name}/log
  handlers_api.go                 # GET /api/builds, GET /api/builds/{id}, GET /api/builds/{id}/steps/{name}/log/stream (SSE)
  templates/
    base.html                     # base layout with Pico.css + HTMX
    builds_list.html              # build list page
    build_detail.html             # build detail + step status
    log_view.html                 # log viewer
  static/
    htmx.min.js                   # vendored HTMX
    pico.min.css                  # vendored Pico.css
```

---

## Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `cmd/cicada/main.go`

- [ ] **Step 1: Initialize Go module**

```bash
go mod init github.com/ianhomer/cicada
```

- [ ] **Step 2: Create the entrypoint**

Create `cmd/cicada/main.go`:

```go
package main

import (
	"flag"
	"log"
	"os"
)

func main() {
	dataDir := flag.String("data-dir", "./data", "directory for SQLite database and log files")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("failed to create data dir: %v", err)
	}

	log.Printf("cicada starting: addr=%s data-dir=%s", *addr, *dataDir)
	// wiring will be added as components are built
}
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./cmd/cicada
```

Expected: binary produced, no errors.

- [ ] **Step 4: Commit**

```bash
git add go.mod cmd/cicada/main.go
git commit -m "feat: project scaffolding and entrypoint"
```

---

## Task 2: Domain Model

**Files:**
- Create: `internal/model/model.go`

- [ ] **Step 1: Write the model**

Create `internal/model/model.go`:

```go
package model

import "time"

// BuildStatus represents the lifecycle state of a build or step.
type BuildStatus string

const (
	StatusPending   BuildStatus = "pending"
	StatusRunning   BuildStatus = "running"
	StatusSuccess   BuildStatus = "success"
	StatusFailure   BuildStatus = "failure"
	StatusCancelled BuildStatus = "cancelled"
)

// EventType represents the kind of forge event that triggered a build.
type EventType string

const (
	EventPush        EventType = "push"
	EventPullRequest EventType = "pull_request"
	EventTag         EventType = "tag"
)

// ForgeEvent is the normalized representation of a webhook event from any forge.
type ForgeEvent struct {
	Type      EventType
	Repo      string
	CloneURL  string
	Ref       string
	CommitSHA string
	Sender    string
}

// Pipeline is parsed from a .cicada/*.yml file at build time. Not stored in DB.
type Pipeline struct {
	Name     string
	Image    string // default image for all steps
	Triggers []EventType
	Steps    []Step
}

// Step is one unit of work in a pipeline.
type Step struct {
	Name      string
	Image     string // resolved at parse time; always non-empty
	Commands  []string
	Env       map[string]string
	DependsOn []string
}

// Build is a single execution of a pipeline, stored in the database.
type Build struct {
	ID           string
	PipelineName string
	Status       BuildStatus
	Ref          string
	CommitSHA    string
	CloneURL     string
	CreatedAt    time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

// StepResult is the result of executing one step in a build, stored in the database.
type StepResult struct {
	BuildID    string
	StepName   string
	Status     BuildStatus
	ExitCode   int
	StartedAt  *time.Time
	FinishedAt *time.Time
	LogFile    string // path relative to data dir, e.g. "logs/<build-id>/<step-name>.log"
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/model/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/model/model.go
git commit -m "feat: domain model types"
```

---

## Task 3: Store — Migrations and SQLite Setup

**Files:**
- Create: `internal/store/store.go`
- Create: `internal/store/migrations/001_initial.sql`

- [ ] **Step 1: Install dependencies**

```bash
go get modernc.org/sqlite
go get github.com/oklog/ulid/v2
```

- [ ] **Step 2: Write the migration SQL**

Create `internal/store/migrations/001_initial.sql`:

```sql
CREATE TABLE IF NOT EXISTS builds (
    id           TEXT PRIMARY KEY,
    pipeline_name TEXT NOT NULL,
    status       TEXT NOT NULL,
    ref          TEXT NOT NULL,
    commit_sha   TEXT NOT NULL,
    clone_url    TEXT NOT NULL,
    created_at   DATETIME NOT NULL,
    started_at   DATETIME,
    finished_at  DATETIME
);

CREATE TABLE IF NOT EXISTS step_results (
    build_id    TEXT NOT NULL,
    step_name   TEXT NOT NULL,
    status      TEXT NOT NULL,
    exit_code   INTEGER NOT NULL DEFAULT 0,
    started_at  DATETIME,
    finished_at DATETIME,
    log_file    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (build_id, step_name),
    FOREIGN KEY (build_id) REFERENCES builds(id)
);
```

- [ ] **Step 3: Write the store setup**

Create `internal/store/store.go`:

```go
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store provides access to the Cicada SQLite database.
type Store struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at dataDir/cicada.db and applies migrations.
func New(dataDir string) (*Store, error) {
	dbPath := filepath.Join(dataDir, "cicada.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite is not safe for concurrent writes

	if err := applyMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func applyMigrations(db *sql.DB) error {
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		data, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if _, err := db.Exec(string(data)); err != nil {
			return fmt.Errorf("exec migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Write the store test**

Create `internal/store/store_test.go`:

```go
package store

import (
	"testing"
)

func TestNew_AppliesMigrations(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer s.Close()

	// Verify both tables exist by querying them.
	for _, table := range []string{"builds", "step_results"} {
		row := s.db.QueryRow("SELECT count(*) FROM " + table)
		var count int
		if err := row.Scan(&count); err != nil {
			t.Errorf("table %s not found or not queryable: %v", table, err)
		}
	}
}
```

- [ ] **Step 5: Run the test**

```bash
go test ./internal/store/...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/store/ go.mod go.sum
git commit -m "feat: SQLite store setup with embedded migrations"
```

---

## Task 4: Store — Build and StepResult CRUD

**Files:**
- Create: `internal/store/builds.go`
- Create: `internal/store/step_results.go`

- [ ] **Step 1: Write build CRUD tests first**

Create `internal/store/builds_test.go`:

```go
package store

import (
	"testing"
	"time"

	"github.com/ianhomer/cicada/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCreateAndGetBuild(t *testing.T) {
	s := newTestStore(t)

	b := &model.Build{
		ID:           "01HTEST00000000000000000001",
		PipelineName: "ci",
		Status:       model.StatusPending,
		Ref:          "refs/heads/main",
		CommitSHA:    "abc123",
		CloneURL:     "https://github.com/example/repo.git",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	}

	if err := s.CreateBuild(b); err != nil {
		t.Fatalf("CreateBuild() error = %v", err)
	}

	got, err := s.GetBuild(b.ID)
	if err != nil {
		t.Fatalf("GetBuild() error = %v", err)
	}

	if got.ID != b.ID {
		t.Errorf("ID = %q, want %q", got.ID, b.ID)
	}
	if got.Status != model.StatusPending {
		t.Errorf("Status = %q, want %q", got.Status, model.StatusPending)
	}
}

func TestUpdateBuildStatus(t *testing.T) {
	s := newTestStore(t)

	b := &model.Build{
		ID:           "01HTEST00000000000000000002",
		PipelineName: "ci",
		Status:       model.StatusPending,
		Ref:          "refs/heads/main",
		CommitSHA:    "abc123",
		CloneURL:     "https://github.com/example/repo.git",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	if err := s.CreateBuild(b); err != nil {
		t.Fatalf("CreateBuild() error = %v", err)
	}

	if err := s.UpdateBuildStatus(b.ID, model.StatusRunning, nil, nil); err != nil {
		t.Fatalf("UpdateBuildStatus() error = %v", err)
	}

	got, err := s.GetBuild(b.ID)
	if err != nil {
		t.Fatalf("GetBuild() error = %v", err)
	}
	if got.Status != model.StatusRunning {
		t.Errorf("Status = %q, want %q", got.Status, model.StatusRunning)
	}
}

func TestListBuilds(t *testing.T) {
	s := newTestStore(t)

	for i, id := range []string{
		"01HTEST00000000000000000010",
		"01HTEST00000000000000000011",
	} {
		b := &model.Build{
			ID:           id,
			PipelineName: "ci",
			Status:       model.StatusPending,
			Ref:          "refs/heads/main",
			CommitSHA:    "abc123",
			CloneURL:     "https://github.com/example/repo.git",
			CreatedAt:    time.Now().UTC().Truncate(time.Second),
		}
		_ = i
		if err := s.CreateBuild(b); err != nil {
			t.Fatalf("CreateBuild() error = %v", err)
		}
	}

	builds, err := s.ListBuilds()
	if err != nil {
		t.Fatalf("ListBuilds() error = %v", err)
	}
	if len(builds) != 2 {
		t.Errorf("len(builds) = %d, want 2", len(builds))
	}
}
```

- [ ] **Step 2: Run the test to see it fail**

```bash
go test ./internal/store/... 2>&1 | head -20
```

Expected: compile error — `CreateBuild`, `GetBuild`, etc. not defined.

- [ ] **Step 3: Implement build CRUD**

Create `internal/store/builds.go`:

```go
package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ianhomer/cicada/internal/model"
)

// CreateBuild inserts a new build record.
func (s *Store) CreateBuild(b *model.Build) error {
	_, err := s.db.Exec(`
		INSERT INTO builds (id, pipeline_name, status, ref, commit_sha, clone_url, created_at, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.PipelineName, string(b.Status), b.Ref, b.CommitSHA, b.CloneURL,
		b.CreatedAt, b.StartedAt, b.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("insert build: %w", err)
	}
	return nil
}

// GetBuild retrieves a build by ID.
func (s *Store) GetBuild(id string) (*model.Build, error) {
	row := s.db.QueryRow(`
		SELECT id, pipeline_name, status, ref, commit_sha, clone_url, created_at, started_at, finished_at
		FROM builds WHERE id = ?`, id)
	return scanBuild(row)
}

// ListBuilds returns all builds ordered by created_at descending.
func (s *Store) ListBuilds() ([]*model.Build, error) {
	rows, err := s.db.Query(`
		SELECT id, pipeline_name, status, ref, commit_sha, clone_url, created_at, started_at, finished_at
		FROM builds ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list builds: %w", err)
	}
	defer rows.Close()

	var builds []*model.Build
	for rows.Next() {
		b, err := scanBuild(rows)
		if err != nil {
			return nil, err
		}
		builds = append(builds, b)
	}
	return builds, rows.Err()
}

// UpdateBuildStatus updates the status and optional timestamps of a build.
func (s *Store) UpdateBuildStatus(id string, status model.BuildStatus, startedAt, finishedAt *time.Time) error {
	_, err := s.db.Exec(`
		UPDATE builds SET status = ?, started_at = ?, finished_at = ? WHERE id = ?`,
		string(status), startedAt, finishedAt, id,
	)
	if err != nil {
		return fmt.Errorf("update build status: %w", err)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanBuild(s scanner) (*model.Build, error) {
	var b model.Build
	var status string
	var startedAt, finishedAt sql.NullTime
	err := s.Scan(
		&b.ID, &b.PipelineName, &status, &b.Ref, &b.CommitSHA, &b.CloneURL,
		&b.CreatedAt, &startedAt, &finishedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan build: %w", err)
	}
	b.Status = model.BuildStatus(status)
	if startedAt.Valid {
		b.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		b.FinishedAt = &finishedAt.Time
	}
	return &b, nil
}
```

- [ ] **Step 4: Write step_result CRUD tests**

Create `internal/store/step_results_test.go`:

```go
package store

import (
	"testing"
	"time"

	"github.com/ianhomer/cicada/internal/model"
)

func TestCreateAndListStepResults(t *testing.T) {
	s := newTestStore(t)

	build := &model.Build{
		ID:           "01HTEST00000000000000000020",
		PipelineName: "ci",
		Status:       model.StatusRunning,
		Ref:          "refs/heads/main",
		CommitSHA:    "abc123",
		CloneURL:     "https://github.com/example/repo.git",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	if err := s.CreateBuild(build); err != nil {
		t.Fatalf("CreateBuild() error = %v", err)
	}

	sr := &model.StepResult{
		BuildID:  build.ID,
		StepName: "test",
		Status:   model.StatusPending,
		LogFile:  "logs/01HTEST00000000000000000020/test.log",
	}
	if err := s.CreateStepResult(sr); err != nil {
		t.Fatalf("CreateStepResult() error = %v", err)
	}

	results, err := s.ListStepResults(build.ID)
	if err != nil {
		t.Fatalf("ListStepResults() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].StepName != "test" {
		t.Errorf("StepName = %q, want %q", results[0].StepName, "test")
	}
}

func TestUpdateStepResult(t *testing.T) {
	s := newTestStore(t)

	build := &model.Build{
		ID:           "01HTEST00000000000000000030",
		PipelineName: "ci",
		Status:       model.StatusRunning,
		Ref:          "refs/heads/main",
		CommitSHA:    "abc123",
		CloneURL:     "https://github.com/example/repo.git",
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	}
	if err := s.CreateBuild(build); err != nil {
		t.Fatalf("CreateBuild() error = %v", err)
	}

	sr := &model.StepResult{
		BuildID:  build.ID,
		StepName: "vet",
		Status:   model.StatusRunning,
		LogFile:  "logs/01HTEST00000000000000000030/vet.log",
	}
	if err := s.CreateStepResult(sr); err != nil {
		t.Fatalf("CreateStepResult() error = %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := s.UpdateStepResult(build.ID, "vet", model.StatusSuccess, 0, nil, &now); err != nil {
		t.Fatalf("UpdateStepResult() error = %v", err)
	}

	results, err := s.ListStepResults(build.ID)
	if err != nil {
		t.Fatalf("ListStepResults() error = %v", err)
	}
	if results[0].Status != model.StatusSuccess {
		t.Errorf("Status = %q, want %q", results[0].Status, model.StatusSuccess)
	}
}
```

- [ ] **Step 5: Implement step_results CRUD**

Create `internal/store/step_results.go`:

```go
package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ianhomer/cicada/internal/model"
)

// CreateStepResult inserts a new step result record.
func (s *Store) CreateStepResult(sr *model.StepResult) error {
	_, err := s.db.Exec(`
		INSERT INTO step_results (build_id, step_name, status, exit_code, started_at, finished_at, log_file)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sr.BuildID, sr.StepName, string(sr.Status), sr.ExitCode,
		sr.StartedAt, sr.FinishedAt, sr.LogFile,
	)
	if err != nil {
		return fmt.Errorf("insert step result: %w", err)
	}
	return nil
}

// ListStepResults returns all step results for a build.
func (s *Store) ListStepResults(buildID string) ([]*model.StepResult, error) {
	rows, err := s.db.Query(`
		SELECT build_id, step_name, status, exit_code, started_at, finished_at, log_file
		FROM step_results WHERE build_id = ?`, buildID)
	if err != nil {
		return nil, fmt.Errorf("list step results: %w", err)
	}
	defer rows.Close()

	var results []*model.StepResult
	for rows.Next() {
		sr, err := scanStepResult(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, sr)
	}
	return results, rows.Err()
}

// UpdateStepResult updates a step result's status, exit code, and timestamps.
func (s *Store) UpdateStepResult(buildID, stepName string, status model.BuildStatus, exitCode int, startedAt, finishedAt *time.Time) error {
	_, err := s.db.Exec(`
		UPDATE step_results SET status = ?, exit_code = ?, started_at = ?, finished_at = ?
		WHERE build_id = ? AND step_name = ?`,
		string(status), exitCode, startedAt, finishedAt, buildID, stepName,
	)
	if err != nil {
		return fmt.Errorf("update step result: %w", err)
	}
	return nil
}

func scanStepResult(rows *sql.Rows) (*model.StepResult, error) {
	var sr model.StepResult
	var status string
	var startedAt, finishedAt sql.NullTime
	err := rows.Scan(
		&sr.BuildID, &sr.StepName, &status, &sr.ExitCode,
		&startedAt, &finishedAt, &sr.LogFile,
	)
	if err != nil {
		return nil, fmt.Errorf("scan step result: %w", err)
	}
	sr.Status = model.BuildStatus(status)
	if startedAt.Valid {
		sr.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		sr.FinishedAt = &finishedAt.Time
	}
	return &sr, nil
}
```

- [ ] **Step 6: Run all store tests**

```bash
go test ./internal/store/...
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/store/
git commit -m "feat: store CRUD for builds and step results"
```

---

## Task 5: Pipeline Parsing and DAG Validation

**Files:**
- Create: `internal/pipeline/parse.go`
- Create: `internal/pipeline/validate.go`
- Create: `internal/pipeline/dag.go`

- [ ] **Step 1: Install YAML dependency**

```bash
go get gopkg.in/yaml.v3
```

- [ ] **Step 2: Write parse tests**

Create `internal/pipeline/parse_test.go`:

```go
package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ianhomer/cicada/internal/model"
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
```

- [ ] **Step 3: Write validate tests**

Create `internal/pipeline/validate_test.go`:

```go
package pipeline

import (
	"testing"

	"github.com/ianhomer/cicada/internal/model"
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
```

- [ ] **Step 4: Run tests to see them fail**

```bash
go test ./internal/pipeline/... 2>&1 | head -20
```

Expected: compile errors — `ParseFile`, `Validate` not defined.

- [ ] **Step 5: Implement ParseFile**

Create `internal/pipeline/parse.go`:

```go
package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ianhomer/cicada/internal/model"
	"gopkg.in/yaml.v3"
)

// rawPipeline is the YAML structure before normalization.
type rawPipeline struct {
	Name     string     `yaml:"name"`
	Image    string     `yaml:"image"`
	On       []string   `yaml:"on"`
	Steps    []rawStep  `yaml:"steps"`
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

// ParseDir reads all *.yml files from a .cicada directory and returns all valid pipelines.
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
```

- [ ] **Step 6: Implement Validate and DAG helpers**

Create `internal/pipeline/validate.go`:

```go
package pipeline

import (
	"fmt"

	"github.com/ianhomer/cicada/internal/model"
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
```

Create `internal/pipeline/dag.go`:

```go
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
```

- [ ] **Step 7: Run all pipeline tests**

```bash
go test ./internal/pipeline/...
```

Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/pipeline/ go.mod go.sum
git commit -m "feat: pipeline YAML parsing and DAG validation"
```

---

## Task 6: Webhook Provider and GitHub Handler

**Files:**
- Create: `internal/webhook/provider.go`
- Create: `internal/webhook/registry.go`
- Create: `internal/webhook/github.go`

- [ ] **Step 1: Write GitHub webhook tests**

Create `internal/webhook/github_test.go`:

```go
package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ianhomer/cicada/internal/model"
)

const testSecret = "testsecret"

func signPayload(t *testing.T, secret, payload string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestGitHubProvider_ParseWebhook_Push(t *testing.T) {
	payload := `{
		"ref": "refs/heads/main",
		"after": "abc123def456",
		"repository": {
			"full_name": "example/repo",
			"clone_url": "https://github.com/example/repo.git"
		},
		"sender": {"login": "octocat"}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(payload))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", signPayload(t, testSecret, payload))

	p := NewGitHubProvider(testSecret)
	event, err := p.ParseWebhook(req)
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}

	if event.Type != model.EventPush {
		t.Errorf("Type = %q, want %q", event.Type, model.EventPush)
	}
	if event.Ref != "refs/heads/main" {
		t.Errorf("Ref = %q, want %q", event.Ref, "refs/heads/main")
	}
	if event.CommitSHA != "abc123def456" {
		t.Errorf("CommitSHA = %q, want %q", event.CommitSHA, "abc123def456")
	}
	if event.CloneURL != "https://github.com/example/repo.git" {
		t.Errorf("CloneURL = %q, want %q", event.CloneURL, "https://github.com/example/repo.git")
	}
}

func TestGitHubProvider_ParseWebhook_Tag(t *testing.T) {
	payload := `{
		"ref": "refs/tags/v1.0.0",
		"after": "deadbeef",
		"repository": {
			"full_name": "example/repo",
			"clone_url": "https://github.com/example/repo.git"
		},
		"sender": {"login": "octocat"}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(payload))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", signPayload(t, testSecret, payload))

	p := NewGitHubProvider(testSecret)
	event, err := p.ParseWebhook(req)
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}
	if event.Type != model.EventTag {
		t.Errorf("Type = %q, want %q", event.Type, model.EventTag)
	}
}

func TestGitHubProvider_ParseWebhook_BadSignature(t *testing.T) {
	payload := `{"ref":"refs/heads/main","after":"abc","repository":{"full_name":"a/b","clone_url":"https://github.com/a/b.git"},"sender":{"login":"u"}}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(payload))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", "sha256=badhash")

	p := NewGitHubProvider(testSecret)
	_, err := p.ParseWebhook(req)
	if err == nil {
		t.Error("ParseWebhook() expected error for bad signature, got nil")
	}
}

func TestGitHubProvider_ParseWebhook_PullRequest(t *testing.T) {
	payload := `{
		"action": "opened",
		"pull_request": {
			"head": {"sha": "pr123sha", "ref": "feature-branch"},
			"base": {"repo": {"clone_url": "https://github.com/example/repo.git", "full_name": "example/repo"}}
		},
		"sender": {"login": "octocat"}
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewBufferString(payload))
	req.Header.Set("X-GitHub-Event", "pull_request")
	req.Header.Set("X-Hub-Signature-256", signPayload(t, testSecret, payload))

	p := NewGitHubProvider(testSecret)
	event, err := p.ParseWebhook(req)
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}
	if event.Type != model.EventPullRequest {
		t.Errorf("Type = %q, want %q", event.Type, model.EventPullRequest)
	}
	if event.CommitSHA != "pr123sha" {
		t.Errorf("CommitSHA = %q, want %q", event.CommitSHA, "pr123sha")
	}
}

func TestRegistryFromEnv(t *testing.T) {
	t.Setenv("CICADA_GITHUB_WEBHOOK_SECRET", "mysecret")
	r := NewRegistryFromEnv()
	p, ok := r.Get("github")
	if !ok {
		t.Fatal("expected github provider in registry")
	}
	if p.Name() != "github" {
		t.Errorf("Name() = %q, want %q", p.Name(), "github")
	}
}

func TestRegistryFromEnv_NoSecret(t *testing.T) {
	os.Unsetenv("CICADA_GITHUB_WEBHOOK_SECRET")
	r := NewRegistryFromEnv()
	_, ok := r.Get("github")
	if ok {
		t.Error("expected github provider to be absent when secret not set")
	}
}
```

- [ ] **Step 2: Run tests to see them fail**

```bash
go test ./internal/webhook/... 2>&1 | head -20
```

Expected: compile errors.

- [ ] **Step 3: Implement provider interface and registry**

Create `internal/webhook/provider.go`:

```go
package webhook

import (
	"net/http"

	"github.com/ianhomer/cicada/internal/model"
)

// ForgeProvider parses raw HTTP webhook requests into normalized ForgeEvents.
type ForgeProvider interface {
	Name() string
	ParseWebhook(r *http.Request) (*model.ForgeEvent, error)
}
```

Create `internal/webhook/registry.go`:

```go
package webhook

import "os"

// Registry holds all registered forge providers.
type Registry struct {
	providers map[string]ForgeProvider
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]ForgeProvider)}
}

// NewRegistryFromEnv creates a registry and registers providers based on environment variables.
// GitHub is registered when CICADA_GITHUB_WEBHOOK_SECRET is set.
func NewRegistryFromEnv() *Registry {
	r := NewRegistry()
	if secret := os.Getenv("CICADA_GITHUB_WEBHOOK_SECRET"); secret != "" {
		r.Register(NewGitHubProvider(secret))
	}
	return r
}

// Register adds a provider to the registry.
func (r *Registry) Register(p ForgeProvider) {
	r.providers[p.Name()] = p
}

// Get retrieves a provider by name.
func (r *Registry) Get(name string) (ForgeProvider, bool) {
	p, ok := r.providers[name]
	return p, ok
}
```

- [ ] **Step 4: Implement GitHub provider**

Create `internal/webhook/github.go`:

```go
package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ianhomer/cicada/internal/model"
)

// GitHubProvider handles GitHub webhook events.
type GitHubProvider struct {
	secret string
}

// NewGitHubProvider creates a GitHub provider with the given webhook secret.
func NewGitHubProvider(secret string) *GitHubProvider {
	return &GitHubProvider{secret: secret}
}

// Name returns the provider's identifier.
func (p *GitHubProvider) Name() string { return "github" }

// ParseWebhook validates the HMAC signature and parses the event payload.
func (p *GitHubProvider) ParseWebhook(r *http.Request) (*model.ForgeEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body)) // restore for potential re-reads

	if err := p.validateSignature(r.Header.Get("X-Hub-Signature-256"), body); err != nil {
		return nil, err
	}

	eventType := r.Header.Get("X-GitHub-Event")
	switch eventType {
	case "push":
		return p.parsePush(body)
	case "pull_request":
		return p.parsePullRequest(body)
	default:
		return nil, fmt.Errorf("unsupported event type: %q", eventType)
	}
}

func (p *GitHubProvider) validateSignature(signature string, body []byte) error {
	if !strings.HasPrefix(signature, "sha256=") {
		return fmt.Errorf("missing or malformed X-Hub-Signature-256 header")
	}
	expected := p.computeHMAC(body)
	actual := strings.TrimPrefix(signature, "sha256=")
	actualBytes, err := hex.DecodeString(actual)
	if err != nil {
		return fmt.Errorf("invalid signature hex: %w", err)
	}
	if !hmac.Equal([]byte(expected), actualBytes) {
		return fmt.Errorf("webhook signature mismatch")
	}
	return nil
}

func (p *GitHubProvider) computeHMAC(body []byte) []byte {
	mac := hmac.New(sha256.New, []byte(p.secret))
	mac.Write(body)
	return mac.Sum(nil)
}

type githubPushPayload struct {
	Ref  string `json:"ref"`
	After string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

func (p *GitHubProvider) parsePush(body []byte) (*model.ForgeEvent, error) {
	var payload githubPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse push payload: %w", err)
	}

	eventType := model.EventPush
	if strings.HasPrefix(payload.Ref, "refs/tags/") {
		eventType = model.EventTag
	}

	return &model.ForgeEvent{
		Type:      eventType,
		Repo:      payload.Repository.FullName,
		CloneURL:  payload.Repository.CloneURL,
		Ref:       payload.Ref,
		CommitSHA: payload.After,
		Sender:    payload.Sender.Login,
	}, nil
}

type githubPRPayload struct {
	PullRequest struct {
		Head struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Repo struct {
				CloneURL string `json:"clone_url"`
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"base"`
	} `json:"pull_request"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
}

func (p *GitHubProvider) parsePullRequest(body []byte) (*model.ForgeEvent, error) {
	var payload githubPRPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse pull_request payload: %w", err)
	}
	return &model.ForgeEvent{
		Type:      model.EventPullRequest,
		Repo:      payload.PullRequest.Base.Repo.FullName,
		CloneURL:  payload.PullRequest.Base.Repo.CloneURL,
		Ref:       payload.PullRequest.Head.Ref,
		CommitSHA: payload.PullRequest.Head.SHA,
		Sender:    payload.Sender.Login,
	}, nil
}
```

- [ ] **Step 5: Run webhook tests**

```bash
go test ./internal/webhook/...
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/webhook/
git commit -m "feat: forge webhook abstraction and GitHub provider"
```

---

## Task 7: Runner — Workspace and Clone

**Files:**
- Create: `internal/runner/runner.go`
- Create: `internal/runner/clone.go`
- Create: `internal/runner/workspace.go`

- [ ] **Step 1: Install Docker SDK**

```bash
go get github.com/docker/docker/client
go get github.com/docker/docker/api/types
go get github.com/docker/docker/api/types/container
go get github.com/docker/docker/api/types/mount
```

- [ ] **Step 2: Write clone test**

Create `internal/runner/clone_test.go`:

```go
package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCloneRepo(t *testing.T) {
	if os.Getenv("CICADA_TEST_DOCKER") == "" {
		t.Skip("set CICADA_TEST_DOCKER=1 to run Docker integration tests")
	}

	dir := t.TempDir()
	// Clone a small public repo at a known commit.
	err := CloneRepo(context.Background(), "https://github.com/git/git.git", "e83c5163316f89bfbde7d9ab23ca2e25604af290", dir)
	if err != nil {
		t.Fatalf("CloneRepo() error = %v", err)
	}

	// The very first git commit should have README.
	if _, err := os.Stat(filepath.Join(dir, "README")); err != nil {
		t.Errorf("expected README in cloned repo: %v", err)
	}
}
```

- [ ] **Step 3: Implement clone**

Create `internal/runner/clone.go`:

```go
package runner

import (
	"context"
	"fmt"
	"os/exec"
)

// CloneRepo clones the given repository URL into destDir and checks out the given commit SHA.
// Uses the system git binary.
func CloneRepo(ctx context.Context, cloneURL, commitSHA, destDir string) error {
	// Shallow clone to minimize bandwidth.
	cmd := exec.CommandContext(ctx, "git", "clone", "--no-checkout", cloneURL, destDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %w\n%s", err, out)
	}

	cmd = exec.CommandContext(ctx, "git", "-C", destDir, "checkout", commitSHA)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %w\n%s", commitSHA, err, out)
	}
	return nil
}
```

- [ ] **Step 4: Write workspace test**

Create `internal/runner/workspace_test.go`:

```go
package runner

import (
	"context"
	"os"
	"testing"
)

func TestWorkspace_CreateAndRemove(t *testing.T) {
	if os.Getenv("CICADA_TEST_DOCKER") == "" {
		t.Skip("set CICADA_TEST_DOCKER=1 to run Docker integration tests")
	}

	ctx := context.Background()
	volumeName, err := CreateWorkspaceVolume(ctx, "cicada-test-ws-"+t.Name())
	if err != nil {
		t.Fatalf("CreateWorkspaceVolume() error = %v", err)
	}
	t.Cleanup(func() {
		if err := RemoveWorkspaceVolume(ctx, volumeName); err != nil {
			t.Logf("cleanup RemoveWorkspaceVolume() error = %v", err)
		}
	})

	if volumeName == "" {
		t.Error("expected non-empty volume name")
	}
}
```

- [ ] **Step 5: Implement workspace helpers**

Create `internal/runner/workspace.go`:

```go
package runner

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

// CreateWorkspaceVolume creates a named Docker volume for sharing files between step containers.
func CreateWorkspaceVolume(ctx context.Context, name string) (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	vol, err := cli.VolumeCreate(ctx, volume.CreateOptions{Name: name})
	if err != nil {
		return "", fmt.Errorf("create docker volume: %w", err)
	}
	return vol.Name, nil
}

// RemoveWorkspaceVolume removes a Docker volume by name.
func RemoveWorkspaceVolume(ctx context.Context, name string) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	if err := cli.VolumeRemove(ctx, name, false); err != nil {
		return fmt.Errorf("remove docker volume: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: Create the runner struct**

Create `internal/runner/runner.go`:

```go
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
func newBuildID() string {
	return ulid.Make().String()
}
```

- [ ] **Step 7: Verify it compiles**

```bash
go build ./internal/runner/...
```

- [ ] **Step 8: Commit**

```bash
git add internal/runner/ go.mod go.sum
git commit -m "feat: runner scaffold, clone, and workspace volume management"
```

---

## Task 8: Runner — Step Executor and DAG Orchestration

**Files:**
- Create: `internal/runner/step_executor.go`
- Create: `internal/runner/dag_executor.go`

- [ ] **Step 1: Write step executor test**

Create `internal/runner/step_executor_test.go`:

```go
package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ianhomer/cicada/internal/model"
)

func TestRunStep_Success(t *testing.T) {
	if os.Getenv("CICADA_TEST_DOCKER") == "" {
		t.Skip("set CICADA_TEST_DOCKER=1 to run Docker integration tests")
	}

	ctx := context.Background()
	logDir := t.TempDir()

	volumeName, err := CreateWorkspaceVolume(ctx, "cicada-test-step-"+t.Name())
	if err != nil {
		t.Fatalf("CreateWorkspaceVolume() error = %v", err)
	}
	defer RemoveWorkspaceVolume(ctx, volumeName)

	step := model.Step{
		Name:     "hello",
		Image:    "alpine:latest",
		Commands: []string{"echo hello world"},
	}

	exitCode, err := RunStep(ctx, step, volumeName, t.TempDir(), logDir)
	if err != nil {
		t.Fatalf("RunStep() error = %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}

	logPath := filepath.Join(logDir, "hello.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected log file at %s: %v", logPath, err)
	}
	if !strings.Contains(string(data), "hello world") {
		t.Errorf("log does not contain 'hello world': %q", string(data))
	}
}

func TestRunStep_Failure(t *testing.T) {
	if os.Getenv("CICADA_TEST_DOCKER") == "" {
		t.Skip("set CICADA_TEST_DOCKER=1 to run Docker integration tests")
	}

	ctx := context.Background()
	logDir := t.TempDir()

	volumeName, err := CreateWorkspaceVolume(ctx, "cicada-test-stepfail-"+t.Name())
	if err != nil {
		t.Fatalf("CreateWorkspaceVolume() error = %v", err)
	}
	defer RemoveWorkspaceVolume(ctx, volumeName)

	step := model.Step{
		Name:     "fail",
		Image:    "alpine:latest",
		Commands: []string{"exit 1"},
	}

	exitCode, err := RunStep(ctx, step, volumeName, t.TempDir(), logDir)
	if err != nil {
		t.Fatalf("RunStep() error = %v", err)
	}
	if exitCode == 0 {
		t.Error("expected non-zero exit code for failing step")
	}
}
```

- [ ] **Step 2: Implement step executor**

Create `internal/runner/step_executor.go`:

```go
package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/ianhomer/cicada/internal/model"
)

// RunStep executes a single pipeline step in a Docker container.
// It streams stdout+stderr to a log file at logDir/<step.Name>.log.
// Returns the container exit code (non-zero means failure) and any system error.
func RunStep(ctx context.Context, step model.Step, volumeName, repoDir, logDir string) (int, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return -1, fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	// Pull image if not present (best-effort; Docker will pull automatically on create if missing).
	cmd := buildCommand(step.Commands)

	env := make([]string, 0, len(step.Env))
	for k, v := range step.Env {
		env = append(env, k+"="+v)
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:      step.Image,
		Cmd:        []string{"sh", "-c", cmd},
		Env:        env,
		WorkingDir: "/workspace",
	}, &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeVolume,
				Source: volumeName,
				Target: "/workspace",
			},
			{
				Type:     mount.TypeBind,
				Source:   repoDir,
				Target:   "/repo",
				ReadOnly: true,
			},
		},
	}, nil, nil, "")
	if err != nil {
		return -1, fmt.Errorf("create container for step %q: %w", step.Name, err)
	}

	containerID := resp.ID
	defer func() {
		cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
	}()

	if err := cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return -1, fmt.Errorf("start container for step %q: %w", step.Name, err)
	}

	// Stream logs to file.
	logFile := filepath.Join(logDir, step.Name+".log")
	if err := streamLogs(ctx, cli, containerID, logFile); err != nil {
		return -1, fmt.Errorf("stream logs for step %q: %w", step.Name, err)
	}

	// Wait for container to exit.
	statusCh, errCh := cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return -1, fmt.Errorf("wait for container (step %q): %w", step.Name, err)
		}
	case status := <-statusCh:
		return int(status.StatusCode), nil
	}
	return -1, fmt.Errorf("unexpected container wait result for step %q", step.Name)
}

func buildCommand(commands []string) string {
	return strings.Join(commands, " && ")
}

func streamLogs(ctx context.Context, cli *client.Client, containerID, logFile string) error {
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("create log file: %w", err)
	}
	defer f.Close()

	logs, err := cli.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	if err != nil {
		return fmt.Errorf("attach logs: %w", err)
	}
	defer logs.Close()

	// Docker multiplexes stdout and stderr; strip the 8-byte header from each frame.
	buf := make([]byte, 8)
	for {
		_, err := io.ReadFull(logs, buf)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read log header: %w", err)
		}
		// buf[4:8] is the payload size (big-endian uint32).
		size := int(buf[4])<<24 | int(buf[5])<<16 | int(buf[6])<<8 | int(buf[7])
		if _, err := io.CopyN(f, logs, int64(size)); err != nil {
			return fmt.Errorf("copy log payload: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 3: Write DAG executor test**

Create `internal/runner/dag_executor_test.go`:

```go
package runner

import (
	"context"
	"os"
	"testing"

	"github.com/ianhomer/cicada/internal/model"
	"github.com/ianhomer/cicada/internal/store"
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
```

- [ ] **Step 4: Implement DAG executor**

Create `internal/runner/dag_executor.go`:

```go
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
		step := step // capture
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(stepDone[step.Name])

			// Wait for all dependencies.
			cancelled := false
			for _, dep := range step.DependsOn {
				<-stepDone[dep]
				statusMu.Lock()
				depStatus := stepStatus[dep]
				statusMu.Unlock()
				if depStatus != model.StatusSuccess {
					cancelled = true
				}
			}

			if cancelled {
				log.Printf("runner: step %q cancelled (dependency failed)", step.Name)
				statusMu.Lock()
				stepStatus[step.Name] = model.StatusCancelled
				statusMu.Unlock()
				r.store.UpdateStepResult(build.ID, step.Name, model.StatusCancelled, 0, nil, nil)
				return
			}

			// Acquire concurrency semaphore.
			sem <- struct{}{}
			defer func() { <-sem }()

			now := time.Now().UTC()
			r.store.UpdateStepResult(build.ID, step.Name, model.StatusRunning, 0, &now, nil)

			log.Printf("runner: executing step %q", step.Name)
			exitCode, err := RunStep(ctx, step, volumeName, repoDir, logDir)
			fin := time.Now().UTC()

			if err != nil {
				log.Printf("runner: step %q system error: %v", step.Name, err)
				statusMu.Lock()
				stepStatus[step.Name] = model.StatusFailure
				statusMu.Unlock()
				r.store.UpdateStepResult(build.ID, step.Name, model.StatusFailure, -1, &now, &fin)
				return
			}

			status := model.StatusSuccess
			if exitCode != 0 {
				status = model.StatusFailure
			}
			statusMu.Lock()
			stepStatus[step.Name] = status
			statusMu.Unlock()
			r.store.UpdateStepResult(build.ID, step.Name, status, exitCode, &now, &fin)
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
```

- [ ] **Step 5: Run all runner tests**

```bash
go test ./internal/runner/...
```

Expected: unit tests PASS; Docker tests skipped (unless `CICADA_TEST_DOCKER=1`).

- [ ] **Step 6: Commit**

```bash
git add internal/runner/
git commit -m "feat: step executor, DAG orchestration, log streaming"
```

---

## Task 9: Scheduler

**Files:**
- Create: `internal/scheduler/scheduler.go`

- [ ] **Step 1: Write scheduler test**

Create `internal/scheduler/scheduler_test.go`:

```go
package scheduler

import (
	"testing"
	"time"

	"github.com/ianhomer/cicada/internal/model"
	"github.com/ianhomer/cicada/internal/store"
)

func TestScheduler_HandleEvent_CreatesBuild(t *testing.T) {
	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	defer s.Close()

	// A runner that just records what it was asked to run.
	var ranBuildID string
	fakeRun := func(build *model.Build) {
		ranBuildID = build.ID
	}

	sched := New(s, fakeRun)

	event := &model.ForgeEvent{
		Type:      model.EventPush,
		Repo:      "example/repo",
		CloneURL:  "https://github.com/example/repo.git",
		Ref:       "refs/heads/main",
		CommitSHA: "abc123",
		Sender:    "octocat",
	}

	if err := sched.HandleEvent(event, "ci"); err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	// Give the goroutine a moment to run.
	time.Sleep(50 * time.Millisecond)

	if ranBuildID == "" {
		t.Error("expected runner to be called, but it was not")
	}

	builds, err := s.ListBuilds()
	if err != nil {
		t.Fatalf("ListBuilds() error = %v", err)
	}
	if len(builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(builds))
	}
	if builds[0].CommitSHA != "abc123" {
		t.Errorf("CommitSHA = %q, want %q", builds[0].CommitSHA, "abc123")
	}
}
```

- [ ] **Step 2: Run test to see it fail**

```bash
go test ./internal/scheduler/... 2>&1 | head -20
```

Expected: compile error — `New` not defined.

- [ ] **Step 3: Implement scheduler**

Create `internal/scheduler/scheduler.go`:

```go
package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ianhomer/cicada/internal/model"
	"github.com/ianhomer/cicada/internal/store"
	"github.com/oklog/ulid/v2"
)

// RunFunc is a function that executes a build. The scheduler calls this in a goroutine.
type RunFunc func(build *model.Build)

// Scheduler receives ForgeEvents, creates Build records, and dispatches them to a runner.
type Scheduler struct {
	store  *store.Store
	runFn  RunFunc
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
	go func() {
		sc.runFn(build)
	}()

	return nil
}

// DispatchEvent parses a ForgeEvent, matches it against available pipelines, and schedules builds.
// pipelineNames is the list of pipeline names available in the repo (determined at webhook time by a lightweight peek).
// For v1 simplicity, the webhook handler passes all known pipeline names and the scheduler creates one build per pipeline
// whose triggers include the event type.
func (sc *Scheduler) DispatchEvent(ctx context.Context, event *model.ForgeEvent, pipelines []scheduledPipeline) error {
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
		if err := sc.HandleEvent(event, p.Name); err != nil {
			log.Printf("scheduler: failed to handle event for pipeline %q: %v", p.Name, err)
		}
	}
	return nil
}

// scheduledPipeline is a lightweight representation of a pipeline used for trigger matching.
type scheduledPipeline struct {
	Name     string
	Triggers []model.EventType
}
```

- [ ] **Step 4: Run scheduler tests**

```bash
go test ./internal/scheduler/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/
git commit -m "feat: scheduler — event handling and build dispatch"
```

---

## Task 10: Web UI — Server, Build List, Build Detail

**Files:**
- Create: `internal/web/server.go`
- Create: `internal/web/handlers_builds.go`
- Create: `internal/web/handlers_webhook.go`
- Create: `internal/web/templates/base.html`
- Create: `internal/web/templates/builds_list.html`
- Create: `internal/web/templates/build_detail.html`
- Create: `internal/web/static/` (downloaded assets)

- [ ] **Step 1: Download static assets**

```bash
mkdir -p internal/web/static
curl -sL https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js -o internal/web/static/htmx.min.js
curl -sL https://cdn.jsdelivr.net/npm/@picocss/pico@2/css/pico.min.css -o internal/web/static/pico.min.css
```

- [ ] **Step 2: Create templates**

Create `internal/web/templates/base.html`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Cicada CI</title>
    <link rel="stylesheet" href="/static/pico.min.css">
    <script src="/static/htmx.min.js"></script>
</head>
<body>
<main class="container">
    <nav>
        <ul><li><strong>Cicada CI</strong></li></ul>
        <ul><li><a href="/builds">Builds</a></li></ul>
    </nav>
    {{block "content" .}}{{end}}
</main>
</body>
</html>
```

Create `internal/web/templates/builds_list.html`:

```html
{{template "base.html" .}}
{{define "content"}}
<h2>Builds</h2>
<div id="builds-list" hx-get="/builds" hx-trigger="every 5s" hx-swap="outerHTML">
    <table>
        <thead>
            <tr>
                <th>ID</th>
                <th>Pipeline</th>
                <th>Ref</th>
                <th>Commit</th>
                <th>Status</th>
                <th>Created</th>
            </tr>
        </thead>
        <tbody>
        {{range .Builds}}
        <tr>
            <td><a href="/builds/{{.ID}}">{{slice .ID 0 8}}</a></td>
            <td>{{.PipelineName}}</td>
            <td>{{.Ref}}</td>
            <td>{{slice .CommitSHA 0 7}}</td>
            <td>{{.Status}}</td>
            <td>{{.CreatedAt.Format "2006-01-02 15:04:05"}}</td>
        </tr>
        {{else}}
        <tr><td colspan="6">No builds yet.</td></tr>
        {{end}}
        </tbody>
    </table>
</div>
{{end}}
```

Create `internal/web/templates/build_detail.html`:

```html
{{template "base.html" .}}
{{define "content"}}
<h2>Build {{slice .Build.ID 0 8}}</h2>
<dl>
    <dt>Pipeline</dt><dd>{{.Build.PipelineName}}</dd>
    <dt>Ref</dt><dd>{{.Build.Ref}}</dd>
    <dt>Commit</dt><dd>{{.Build.CommitSHA}}</dd>
    <dt>Status</dt><dd id="build-status" hx-get="/api/builds/{{.Build.ID}}" hx-trigger="every 3s" hx-swap="innerHTML" hx-select="#build-status-inner"><span id="build-status-inner">{{.Build.Status}}</span></dd>
    <dt>Created</dt><dd>{{.Build.CreatedAt.Format "2006-01-02 15:04:05"}}</dd>
</dl>
<h3>Steps</h3>
<table>
    <thead><tr><th>Name</th><th>Status</th><th>Exit Code</th><th>Log</th></tr></thead>
    <tbody>
    {{range .Steps}}
    <tr>
        <td>{{.StepName}}</td>
        <td>{{.Status}}</td>
        <td>{{.ExitCode}}</td>
        <td><a href="/builds/{{$.Build.ID}}/steps/{{.StepName}}/log">log</a></td>
    </tr>
    {{end}}
    </tbody>
</table>
{{end}}
```

Create `internal/web/templates/log_view.html`:

```html
{{template "base.html" .}}
{{define "content"}}
<h2>Log: {{.StepName}}</h2>
<p><a href="/builds/{{.BuildID}}">← back to build</a></p>
<pre id="log-output">{{.LogContent}}</pre>
{{if .Live}}
<div hx-ext="sse" sse-connect="/api/builds/{{.BuildID}}/steps/{{.StepName}}/log/stream">
    <div sse-swap="message" hx-swap="beforeend" hx-target="#log-output"></div>
</div>
{{end}}
{{end}}
```

- [ ] **Step 3: Implement server and build handlers**

Create `internal/web/server.go`:

```go
package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/ianhomer/cicada/internal/scheduler"
	"github.com/ianhomer/cicada/internal/store"
	"github.com/ianhomer/cicada/internal/webhook"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server holds dependencies for the HTTP server.
type Server struct {
	store     *store.Store
	registry  *webhook.Registry
	scheduler *scheduler.Scheduler
	tmpl      *template.Template
	mux       *http.ServeMux
}

// New creates a Server and registers all routes.
func New(s *store.Store, registry *webhook.Registry, sched *scheduler.Scheduler) (*Server, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	srv := &Server{
		store:     s,
		registry:  registry,
		scheduler: sched,
		tmpl:      tmpl,
		mux:       http.NewServeMux(),
	}
	srv.registerRoutes()
	return srv, nil
}

func (srv *Server) registerRoutes() {
	staticContent, _ := fs.Sub(staticFS, "static")
	srv.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticContent))))

	srv.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/builds", http.StatusFound)
	})
	srv.mux.HandleFunc("GET /builds", srv.handleBuildsList)
	srv.mux.HandleFunc("GET /builds/{id}", srv.handleBuildDetail)
	srv.mux.HandleFunc("GET /builds/{id}/steps/{name}/log", srv.handleLogView)
	srv.mux.HandleFunc("POST /webhooks/{provider}", srv.handleWebhook)
	srv.mux.HandleFunc("GET /api/builds", srv.handleAPIBuilds)
	srv.mux.HandleFunc("GET /api/builds/{id}", srv.handleAPIBuild)
	srv.mux.HandleFunc("GET /api/builds/{id}/steps/{name}/log/stream", srv.handleLogStream)
}

// ServeHTTP implements http.Handler.
func (srv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	srv.mux.ServeHTTP(w, r)
}
```

Create `internal/web/handlers_builds.go`:

```go
package web

import (
	"net/http"
)

func (srv *Server) handleBuildsList(w http.ResponseWriter, r *http.Request) {
	builds, err := srv.store.ListBuilds()
	if err != nil {
		http.Error(w, "failed to list builds", http.StatusInternalServerError)
		return
	}
	data := map[string]any{"Builds": builds}
	if err := srv.tmpl.ExecuteTemplate(w, "builds_list.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (srv *Server) handleBuildDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	build, err := srv.store.GetBuild(id)
	if err != nil {
		http.Error(w, "build not found", http.StatusNotFound)
		return
	}
	steps, err := srv.store.ListStepResults(id)
	if err != nil {
		http.Error(w, "failed to list steps", http.StatusInternalServerError)
		return
	}
	data := map[string]any{"Build": build, "Steps": steps}
	if err := srv.tmpl.ExecuteTemplate(w, "build_detail.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

- [ ] **Step 4: Implement webhook handler and update scheduler for pipeline discovery**

The webhook handler clones the repo in a goroutine, discovers which pipelines match the event trigger, and dispatches one build per matching pipeline.

First, add `DispatchForgeEvent` to `internal/scheduler/scheduler.go`. Add these imports to the existing file: `"context"`, `"path/filepath"`, `"github.com/ianhomer/cicada/internal/pipeline"`. Then add the method:

```go
// DispatchForgeEvent clones the repo, discovers pipelines matching the event trigger,
// creates a build for each, and dispatches them to the runner.
func (sc *Scheduler) DispatchForgeEvent(ctx context.Context, event *model.ForgeEvent, cloneDir string) error {
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
```

Add required imports to `scheduler.go`: `"path/filepath"`, `"github.com/ianhomer/cicada/internal/pipeline"`.

Update `internal/web/handlers_webhook.go` to call `DispatchForgeEvent`:

```go
package web

import (
	"log"
	"net/http"
	"os"

	"github.com/ianhomer/cicada/internal/runner"
)

func (srv *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	providerName := r.PathValue("provider")
	provider, ok := srv.registry.Get(providerName)
	if !ok {
		http.Error(w, "unknown provider", http.StatusNotFound)
		return
	}

	event, err := provider.ParseWebhook(r)
	if err != nil {
		log.Printf("webhook: parse error for %s: %v", providerName, err)
		http.Error(w, "invalid webhook", http.StatusBadRequest)
		return
	}

	log.Printf("webhook: received %s event from %s (ref=%s sha=%s)", event.Type, event.Repo, event.Ref, event.CommitSHA)

	go func() {
		// Clone the repo to discover pipelines and trigger builds.
		cloneDir, err := os.MkdirTemp("", "cicada-discover-*")
		if err != nil {
			log.Printf("webhook: failed to create temp dir: %v", err)
			return
		}
		defer os.RemoveAll(cloneDir)

		if err := runner.CloneRepo(r.Context(), event.CloneURL, event.CommitSHA, cloneDir); err != nil {
			log.Printf("webhook: clone failed: %v", err)
			return
		}

		if err := srv.scheduler.DispatchForgeEvent(r.Context(), event, cloneDir); err != nil {
			log.Printf("webhook: dispatch failed: %v", err)
		}
	}()

	w.WriteHeader(http.StatusNoContent)
}
```

Also remove the now-unused `DispatchEvent` method and unexported `scheduledPipeline` type from `internal/scheduler/scheduler.go`. Keep `HandleEvent` — the scheduler test uses it directly. The final `scheduler.go` should export only `Scheduler`, `RunFunc`, `New`, `HandleEvent`, and `DispatchForgeEvent`.

- [ ] **Step 6: Implement API and log handlers**

Create `internal/web/handlers_api.go`:

```go
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func (srv *Server) handleAPIBuilds(w http.ResponseWriter, r *http.Request) {
	builds, err := srv.store.ListBuilds()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(builds)
}

func (srv *Server) handleAPIBuild(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	build, err := srv.store.GetBuild(id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(build)
}

func (srv *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	buildID := r.PathValue("id")
	stepName := r.PathValue("name")

	results, err := srv.store.ListStepResults(buildID)
	if err != nil || len(results) == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var logFile string
	for _, sr := range results {
		if sr.StepName == stepName {
			logFile = sr.LogFile
			break
		}
	}
	if logFile == "" {
		http.Error(w, "step not found", http.StatusNotFound)
		return
	}

	absPath := filepath.Join(srv.store.DataDir(), logFile)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	f, err := os.Open(absPath)
	if err != nil {
		// File may not exist yet if step hasn't started.
		fmt.Fprintf(w, "data: (waiting for log)\n\n")
		flusher.Flush()
		return
	}
	defer f.Close()

	buf := make([]byte, 4096)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			n, _ := f.Read(buf)
			if n > 0 {
				fmt.Fprintf(w, "data: %s\n\n", buf[:n])
				flusher.Flush()
			}
		}
	}
}
```

Create `internal/web/handlers_log.go`:

```go
package web

import (
	"net/http"
	"os"
	"path/filepath"
)

func (srv *Server) handleLogView(w http.ResponseWriter, r *http.Request) {
	buildID := r.PathValue("id")
	stepName := r.PathValue("name")

	results, err := srv.store.ListStepResults(buildID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var logFile string
	var live bool
	for _, sr := range results {
		if sr.StepName == stepName {
			logFile = sr.LogFile
			live = sr.Status == "running" || sr.Status == "pending"
			break
		}
	}
	if logFile == "" {
		http.Error(w, "step not found", http.StatusNotFound)
		return
	}

	absPath := filepath.Join(srv.store.DataDir(), logFile)
	content, _ := os.ReadFile(absPath) // best-effort; empty if not yet written

	data := map[string]any{
		"BuildID":    buildID,
		"StepName":   stepName,
		"LogContent": string(content),
		"Live":       live,
	}
	if err := srv.tmpl.ExecuteTemplate(w, "log_view.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
```

**Note:** `srv.store.DataDir()` requires adding a `DataDir() string` method to the `Store`. Add it to `internal/store/store.go`:

```go
// DataDir returns the data directory path.
func (s *Store) DataDir() string {
	return s.dataDir
}
```

Also update `Store` struct to store `dataDir`:

```go
type Store struct {
	db      *sql.DB
	dataDir string
}
```

And update `New()` to set it:

```go
return &Store{db: db, dataDir: dataDir}, nil
```

- [ ] **Step 7: Wire everything in main.go**

Update `cmd/cicada/main.go`:

```go
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ianhomer/cicada/internal/model"
	"github.com/ianhomer/cicada/internal/runner"
	"github.com/ianhomer/cicada/internal/scheduler"
	"github.com/ianhomer/cicada/internal/store"
	"github.com/ianhomer/cicada/internal/web"
	"github.com/ianhomer/cicada/internal/webhook"
)

func main() {
	dataDir := flag.String("data-dir", "./data", "directory for SQLite database and log files")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	s, err := store.New(*dataDir)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer s.Close()

	r := runner.New(s, *dataDir)
	runFn := func(build *model.Build) {
		if err := r.Run(context.Background(), build); err != nil {
			log.Printf("runner error: %v", err)
		}
	}

	sched := scheduler.New(s, runFn)
	registry := webhook.NewRegistryFromEnv()

	srv, err := web.New(s, registry, sched)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}

	httpSrv := &http.Server{
		Addr:    *addr,
		Handler: srv,
	}

	go func() {
		log.Printf("cicada listening on %s", *addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	httpSrv.Shutdown(ctx)
}
```

- [ ] **Step 8: Build the full binary**

```bash
go build ./...
```

Fix any remaining compile errors. Common issues:
- Import cycles (e.g. `web` importing `runner` for `CloneRepo` — acceptable since they're in separate packages)
- Missing `Store.DataDir()` method (added in step 6)
- Type mismatches in `runFn` signature (`*model.Build`, not `*runner.Build`)

- [ ] **Step 9: Commit**

```bash
git add internal/web/ internal/scheduler/ internal/store/ cmd/cicada/ go.mod go.sum
git commit -m "feat: web UI, HTTP server, webhook handler, API endpoints, SSE log streaming"
```

---

## Task 11: End-to-End Smoke Test and Dogfood Pipeline

**Files:**
- Create: `.cicada/ci.yml`

- [ ] **Step 1: Run the full test suite**

```bash
go test ./...
```

Expected: all unit tests PASS. Integration tests skipped.

- [ ] **Step 2: Run the binary locally**

```bash
go build -o cicada ./cmd/cicada && ./cicada --addr :8080 --data-dir ./data
```

Open `http://localhost:8080/builds` in a browser. Verify the empty build list renders.

- [ ] **Step 3: Write the dogfood pipeline**

Create `.cicada/ci.yml`:

```yaml
name: ci
image: golang:1.22
on:
  - push
  - pull_request
steps:
  - name: vet
    commands:
      - go vet ./...
  - name: test
    commands:
      - go test ./...
    dependsOn:
      - vet
  - name: build
    commands:
      - go build ./cmd/cicada
    dependsOn:
      - test
```

- [ ] **Step 4: Validate pipeline parses correctly**

```bash
go test ./internal/pipeline/... -run TestParseFile
```

Also write a quick ad-hoc check:

```bash
go run ./cmd/cicada --addr :9999 &
# Manually verify the server starts
kill %1
```

- [ ] **Step 5: Commit**

```bash
git add .cicada/ci.yml
git commit -m "feat: dogfood CI pipeline for Cicada itself"
```

---

## Task 12: Final Polish and README

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write a minimal README**

Create `README.md`:

```markdown
# Cicada

A self-hosted CI/CD platform. Receives GitHub webhooks, runs pipeline steps in Docker containers.

## Requirements

- Go 1.22+
- Docker

## Quick Start

```bash
go build -o cicada ./cmd/cicada
CICADA_GITHUB_WEBHOOK_SECRET=yoursecret ./cicada --addr :8080 --data-dir ./data
```

Configure GitHub to POST webhooks to `http://your-server:8080/webhooks/github`.
For local development, use [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/) or [ngrok](https://ngrok.com) to expose your local port.

## Pipeline Configuration

Add `.cicada/*.yml` files to your repository:

```yaml
name: ci
image: golang:1.22
on:
  - push
  - pull_request
steps:
  - name: test
    commands:
      - go test ./...
```

## Running Tests

```bash
go test ./...                          # unit tests
CICADA_TEST_DOCKER=1 go test ./...     # includes Docker integration tests
```
```

- [ ] **Step 2: Final build verification**

```bash
go build ./... && go test ./...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add README with quick start and pipeline configuration"
```
