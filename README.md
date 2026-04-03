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

Open `http://localhost:8080/builds` to view the build dashboard.

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
