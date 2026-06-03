# Repository Guidelines

## Project Structure & Module Organization

This repository is a Go service rooted at `github.com/chistopat/fsqr`. The API entry point is `cmd/fsqr`; application code is under `internal/` for HTTP, services, repositories, models, config, logging, embeddings, and observability. SQL migrations are in `migrations/`, with repository query files beside code in `internal/repository/**`. Integration and e2e tests live in `tests/` with fixtures in `tests/fixtures/`. Docker files are in `build/`, deployment infrastructure in `deployment/`, configs in `config/`, OpenAPI in `api/openapi.yaml`, observability assets in `observability/`, and Python ETL tooling in `etl/`.

## Build, Test, and Development Commands

Use `just` as the main task runner:

- `just service-build` builds `bin/fsqr` from `./cmd/fsqr`.
- `just lint` runs `golangci-lint` v2.8.0 with the repository config.
- `just lint-fix` applies supported lint fixes.
- `just pre-commit` builds, lints, runs `go test ./...`, and runs `go vet ./...`.
- `just up` starts the e2e Postgres, TEI, and API stack.
- `just test-e2e` starts services, migrates, waits, then runs `go test -tags=e2e -count=1 -v ./tests`.
- `just down` stops the e2e compose stack.

For quick unit checks, run `go test ./...`.

## Coding Style & Naming Conventions

Write idiomatic Go formatted with `gofmt` and `goimports`; CI rejects unformatted files. Keep packages lowercase and focused by layer. Use `*_test.go` files and descriptive names such as `TestPlaceSearchRanksCategoryMatches`. Respect `.golangci.yml`: line length is 120, functions should stay under 120 lines and 50 statements, and architecture guards keep HTTP, repository, and service dependencies separated. `nolint` comments must name the linter and explain why.

## Testing Guidelines

Unit tests are colocated with code in `internal/**`; broader API/database tests are in `tests/`. E2E tests require Docker Compose, Postgres, and TEI via `just test-e2e`. Update SQL fixtures in `tests/fixtures/` when repository behavior depends on seeded data. Release script tests run with `python3 -m unittest scripts/next_release_test.py`.

## Commit & Pull Request Guidelines

Recent history uses short imperative commit messages, sometimes with a conventional prefix, for example `Fix production config file bootstrap` and `ci: create GitHub releases for published images`. Keep commits focused and mention the affected area when helpful. Pull requests should describe the change, list validation commands, link issues, and include screenshots only for visible API docs, dashboards, or UI-facing assets.

## Security & Configuration Tips

Do not commit secrets from `.env`, deployment tokens, or generated private keys. Keep environment-specific settings in `config/*.yaml` and deployment variables in `deployment/variables.tf`. Prefer `just` recipes so required environment checks run consistently.
