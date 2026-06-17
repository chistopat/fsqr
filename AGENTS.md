# Repository Guidelines

## Project Structure & Module Organization

This repository is the Kailas monorepo. Applications live under `apps/`; shared runtime and infrastructure live at the repository root.

- `apps/fsqr` is the existing Go service with module path `github.com/chistopat/fsqr`. Its entry point is `apps/fsqr/cmd/fsqr`, application code is in `apps/fsqr/internal`, OpenAPI files are in `apps/fsqr/api`, config examples are in `apps/fsqr/config`, SQL migrations are in `apps/fsqr/migrations`, and e2e tests plus fixtures are in `apps/fsqr/tests`.
- `apps/hoppify` is a single-container Go app with module path `github.com/chistopat/hoppify`. Its entry point is `apps/hoppify/cmd/hoppify`; it serves API routes and static frontend assets from `apps/hoppify/internal/http/web`.
- `gateway/` contains Caddy configuration.
- `deployment/` contains Docker Compose, OpenTofu/Terraform, cloud-init, and the shared `kailas-goose` Dockerfile.
- `observability/` contains Prometheus and Grafana provisioning assets.
- `etl/` contains Python ETL tooling for fsqr data imports.

Keep application code inside its app directory. Shared deployment changes belong in `deployment/`, `gateway/`, or `observability/`, not inside an app package.

## Build, Test, and Development Commands

Use `just` as the main task runner:

- `just service-build` builds `bin/fsqr` and `bin/hoppify`.
- `just lint` runs `golangci-lint` v2.8.0 for fsqr and Hoppify.
- `just lint-fix` applies supported lint fixes for fsqr and Hoppify.
- `just pre-commit` builds both apps, lints both apps, runs unit tests, and runs `go vet`.
- `just up` starts the e2e Postgres, TEI, fsqr, and Hoppify stack.
- `just test-e2e` starts services, migrates fsqr, waits, then runs fsqr e2e tests from `apps/fsqr`.
- `just down` stops the e2e compose stack.
- `just bootstrap` syncs the production compose bundle to Hetzner and starts the shared stack.
- `just migrate` syncs fsqr migrations and runs `migrate-fsqr` on the server.

For app-local checks:

```sh
cd apps/fsqr && go test ./...
cd apps/hoppify && go test ./...
```

For compose validation:

```sh
FSQR_DOMAIN=fsqr.example.test HOPPIFY_DOMAIN=hoppify.example.test GRAFANA_DOMAIN=grafana.example.test POSTGRES_PASSWORD=fsqr GRAFANA_ADMIN_PASSWORD=admin docker compose -f deployment/compose.prod.yml config
```

## Coding Style & Naming Conventions

Write idiomatic Go formatted with `gofmt` and `goimports`; CI rejects unformatted files. Keep packages lowercase and focused by layer. Use descriptive test names such as `TestPlaceSearchRanksCategoryMatches` or `TestHandlerServesPlaceholderPage`.

Respect `.golangci.yml`: line length is 120, functions should stay under 120 lines and 50 statements, and architecture guards keep HTTP, repository, and service dependencies separated. `nolint` comments must name the linter and explain why.

## Testing Guidelines

Unit tests are colocated with each app. Fsqr unit tests live under `apps/fsqr/internal/**`; broader fsqr API/database tests are in `apps/fsqr/tests`. Hoppify tests live under `apps/hoppify/internal/**`.

E2E tests require Docker Compose, Postgres, and TEI via `just test-e2e`. Update SQL fixtures in `apps/fsqr/tests/fixtures/` when fsqr repository behavior depends on seeded data. Release script tests run with:

```sh
python3 -m unittest scripts/next_release_test.py
```

## Deployment Model

Production is one Hetzner Compose project. Caddy, Postgres, TEI, Prometheus, Grafana, Watchtower, fsqr, and Hoppify run in the same stack. Watchtower updates application containers that carry `com.centurylinklabs.watchtower.enable=true`.

The shared goose image is `ghcr.io/chistopat/kailas-goose:v3.27.1`. Fsqr migrations run through the `migrate-fsqr` service. Hoppify currently has no database migrations.

Keep the production compose project name stable unless explicitly migrating Docker volumes. The default remains `fsqr-prod` for compatibility with the existing server volumes.

## Commit & Pull Request Guidelines

Recent history uses short imperative commit messages, sometimes with a conventional prefix, for example `Fix production config file bootstrap` and `ci: create GitHub releases for published images`. Keep commits focused and mention the affected area when helpful.

Pull requests should describe the change, list validation commands, link issues, and include screenshots only for visible API docs, dashboards, or UI-facing assets.

## Security & Configuration Tips

Do not commit secrets from `.env`, deployment tokens, or generated private keys. Keep app-specific example config in the owning app directory. Keep deployment variables in `deployment/variables.tf`.

Production domains are sourced from root `.env`, Terraform variables, GitHub Actions repository variables, and the generated server `.env`. Keep these aligned:

- `FSQR_DOMAIN`
- `HOPPIFY_DOMAIN`
- `GRAFANA_DOMAIN`
- `FSQR_DNS_ZONE`

Prefer `just` recipes so required environment checks run consistently.
