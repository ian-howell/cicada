# Cicada Design Spec

**Date:** 2026-04-03  
**Status:** Approved

## Overview

Cicada is a self-hosted CI/CD platform for personal and small team use. It receives webhooks from code forges (GitHub first), clones repositories, parses pipeline definitions from YAML files, and executes build steps in Docker containers. A web UI provides build monitoring with live log streaming.

**Goals:** Simple to deploy (single binary), simple to configure (YAML in-repo), reliable for small-scale use.

**Non-goals for v1:** Multi-tenancy, horizontal scaling, plugin system, matrix builds, secrets management, log retention policies.

## Architecture

Single Go binary with a flat-ish monolithic package structure. Internal communication via direct function calls and Go channels. Deployed as a single process; users behind firewalls document tunnel setup (Cloudflare Tunnel or ngrok).

```
cicada/
  cmd/cicada/         # main entrypoint, flag parsing, wiring
  internal/
    model/            # domain types
    store/            # SQLite data access
    runner/           # Docker execution engine; owns DAG execution logic
    scheduler/        # build queue: accepts ForgeEvents, queues builds, dispatches to runner
    webhook/          # forge webhook handlers; parses requests into ForgeEvents
    web/              # HTTP server, templates, HTMX handlers
    pipeline/         # YAML parsing, validation, DAG graph construction
  data/               # runtime default: SQLite DB + logs (configurable via --data-dir)
```

**Router:** Go 1.22+ standard library `http.ServeMux` — no third-party router.

## 1. Domain Model

**Pipeline** (parsed from YAML at build time; not stored in DB):
- `Name string` — derived from filename
- `Image string` — default Docker image for all steps (optional if all steps set their own)
- `Triggers []EventType`
- `Steps []Step`

**Step:**
- `Name string`
- `Image string` — overrides pipeline-level image (resolved at parse time; runner always sees a concrete image)
- `Commands []string`
- `Env map[string]string`
- `DependsOn []string` — names of steps this step depends on (DAG)

**Build** (stored in DB):
- `ID string` — ULID
- `PipelineName string`
- `Status BuildStatus`
- `Ref string` — branch or tag name
- `CommitSHA string`
- `CloneURL string`
- `CreatedAt, StartedAt, FinishedAt time.Time`

**StepResult** (stored in DB):
- `BuildID string`
- `StepName string`
- `Status BuildStatus`
- `ExitCode int`
- `StartedAt, FinishedAt time.Time`
- `LogFile string` — path relative to the data dir (e.g. `logs/<build-id>/<step-name>.log`)

**BuildStatus** (string enum): `pending`, `running`, `success`, `failure`, `cancelled`

**EventType** (string enum): `push`, `pull_request`, `tag`

Pipeline definitions are read from the repository at build time, not stored in the database.  
IDs use ULIDs.

## 2. YAML Schema

Pipeline files live at `.cicada/*.yml` in the repository. Multiple pipeline files are supported; each file defines one pipeline.

```yaml
name: ci

image: golang:1.22  # default for all steps; can be overridden per step

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
    depends_on:
      - vet

  - name: build
    image: golang:1.22-alpine  # step-level override
    commands:
      - go build ./cmd/cicada
    depends_on:
      - test
```

**Validation rules:**
- Every step must have a concrete image — either via the pipeline-level `image` or its own `image` field.
- Step names must be unique within a pipeline.
- `depends_on` references must name steps that exist in the same pipeline.
- Circular dependencies are rejected at parse time.

Images are resolved at parse time so the runner always sees a concrete image per step.

## 3. Runner & Docker Execution

**Build lifecycle:**
1. Clone repository to a temporary directory
2. Parse `.cicada/*.yml` files
3. Create a Docker volume for the workspace (mounted at `/workspace` in all containers)
4. Execute steps according to the DAG
5. Clean up: remove workspace volume and temp directory

**DAG execution:**
- Steps with no dependencies start immediately.
- A step starts when all its `depends_on` steps have succeeded.
- If a step fails, downstream dependents are marked `cancelled` (skipped), not retried.
- Steps with no shared dependencies run concurrently.
- A global concurrency semaphore (buffered channel, default capacity = number of CPU cores) caps simultaneous container executions.

**Container execution:**
- Docker Go SDK (`github.com/docker/docker/client`) — no shelling out to the Docker CLI.
- Commands are wrapped: `sh -c "cmd1 && cmd2 && cmd3"`.
- Container stdout+stderr is streamed to disk in real-time: `data/logs/<build-id>/<step-name>.log`.
- Each step container mounts the shared workspace volume at `/workspace`.

## 4. Storage & Data Layer

**SQLite database:** `<data-dir>/cicada.db`  
**Log files:** `<data-dir>/logs/<build-id>/<step-name>.log`  
**Default data dir:** `./data`, configurable via `--data-dir` flag.

**Tables:** `builds`, `step_results`

**Stack:**
- Plain `database/sql` with the `modernc.org/sqlite` driver (pure Go, no CGo dependency).
- SQL migrations embedded via `go:embed` and applied at startup.
- No ORM.

No log retention policy in v1.

## 5. Webhook & Forge Abstraction

**ForgeProvider interface:**
```go
type ForgeProvider interface {
    Name() string
    ParseWebhook(r *http.Request) (*ForgeEvent, error)
}
```

**ForgeEvent:**
```go
type ForgeEvent struct {
    Type      EventType
    Repo      string
    CloneURL  string
    Ref       string
    CommitSHA string
    Sender    string
}
```

**Provider registry:** `map[string]ForgeProvider`, keyed by provider name (e.g. `"github"`).

**GitHub implementation:**
- Validates `X-Hub-Signature-256` HMAC on every request.
- Parses `push` and `pull_request` event payloads. Tag pushes arrive as GitHub `push` events with a `refs/tags/` ref and are classified as `EventType` `tag`.
- Webhook secret configured via `CICADA_GITHUB_WEBHOOK_SECRET` environment variable.

Webhook endpoint: `POST /webhooks/{provider}`

## 6. Web UI & HTTP Server

**Routes:**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/webhooks/{provider}` | Forge webhook receiver |
| `GET` | `/` | Redirect to `/builds` |
| `GET` | `/builds` | Build list (HTMX polling) |
| `GET` | `/builds/{id}` | Build detail + step status |
| `GET` | `/builds/{id}/steps/{name}/log` | Raw or streaming log view |
| `GET` | `/api/builds` | JSON build list |
| `GET` | `/api/builds/{id}` | JSON build detail |
| `GET` | `/api/builds/{id}/steps/{name}/log/stream` | SSE live log stream |

**Templating:** `html/template`, embedded in binary via `go:embed`.

**Interactivity:**
- HTMX for dynamic updates — no custom JavaScript.
- Build list auto-refreshes via HTMX polling.
- Live log output via Server-Sent Events (SSE); HTMX's `hx-ext="sse"` handles the subscription.
- Build detail page uses SSE to update step statuses in real-time.

**Styling:** Minimal classless CSS (Pico.css), embedded in binary.

**Static assets:** HTMX JS and CSS embedded via `go:embed`. No build step, no Node.js.

## 7. Testing Strategy

**Unit tests** (`go test ./...`):
- `pipeline/` — YAML parsing, validation, DAG resolution (valid configs, missing images, circular deps)
- `model/` — domain logic, status types
- `webhook/` — payload parsing, signature validation using known test payloads
- `scheduler/` — DAG ordering, failure propagation
- `store/` — tested against a real in-memory SQLite (`:memory:`), not mocked; migrations run in test setup

**Integration tests** (`go test -tags=integration ./...`):
- Tagged with `//go:build integration` to keep `go test ./...` fast.
- `runner/` — actually spin up containers, run commands, verify exit codes and log output.
- End-to-end: webhook receipt → clone → parse → execute → verify build status in DB.
- Use small test configs in `testdata/` with known `.cicada/*.yml` files.

**Test helpers:** Written in `_test.go` files alongside the tests that use them (e.g. in-memory DB setup in `store/`, signed webhook request builder in `webhook/`). No shared `testutil` package — extract one only if real duplication emerges across packages.

**No mocking framework** — hand-write test doubles using interfaces where needed.

**Dogfooding:**  
Once Cicada is functional, add `.cicada/ci.yml` to the Cicada repository itself. Initial pipeline:
- `go vet ./...`
- `go test ./...` (depends on vet)
- `go build ./cmd/cicada` (depends on test)
- `go test -tags=integration ./...` as a separate step depending on unit tests

The pipeline YAML can be written early and validated manually before Cicada can run it itself. Dogfooding is a milestone goal, not a v1 blocker.

## Configuration & Startup

Configured via CLI flags and environment variables:

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--data-dir` | — | `./data` | Directory for SQLite DB and log files |
| `--addr` | — | `:8080` | HTTP listen address |
| — | `CICADA_GITHUB_WEBHOOK_SECRET` | — | GitHub webhook HMAC secret |

At startup: apply DB migrations, register forge providers, start HTTP server.
