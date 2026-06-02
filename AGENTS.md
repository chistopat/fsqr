# AGENTS.md

## Scope

These instructions apply to the whole repository.

This repository is `fsqr`: a Go/Fiber semantic geosearch API backed by
Postgres, pgvector, and a TEI/OpenAI-compatible embedding endpoint.

The API contract source of truth is `api/openapi.yaml`. Keep implementation,
tests, and examples aligned with that file.

## Current Architecture

- Entry point: `cmd/fsqr/main.go`.
- HTTP layer: `internal/http`, currently Fiber routes for `/api/v1/search`,
  `/api/v1/categories`, `/api/v1/places/:uuid`, plus `/metrics`.
- Service layer packages:
  - `internal/service/search`: helper implementations for search normalization,
    scoring, and ranking. Keep them split into focused subpackages such as
    `normalizer`, `ranker`, and `scorer`.
  - `internal/service/category`: category retrieval service.
  - `internal/service/place`: place search and place details service.
- Embedding client: `internal/embeddings/openai.go`.
- Repositories: `internal/repository/categories` and
  `internal/repository/places`.
- Data model and validation: `internal/models`.
- SQL migrations: `migrations`.
- ETL/import tooling: `etl`.
- Local orchestration: `build/docker-compose.yml`,
  `build/docker-compose.e2e.yml`, and `justfile`.
- Functional e2e tests: `tests/*.go` behind the `e2e` build tag. Keep them
  directly under `tests`; do not add a nested `tests/e2e` package.
- Functional test plan and fixtures: `docs/test-cases/cases.md` and
  `docs/test-cases/fixtures`.

Primary request flow for `GET /api/v1/search`:

1. HTTP parses `query`, `location`, `limit`, and `distance_meters`.
2. `PlaceSearch.SearchPlaces` validates input.
3. `category.Service.SearchCategories` runs FTS and embedding/vector search
   concurrently.
4. Results are min-max normalized, RRF-scored, rank-sorted, filtered by relative
   score, and capped.
5. The top category IDs, capped at 3, drive place search.
6. Place search builds a bounding box and queries Postgres. Normal bounding
   boxes use GiST/KNN over `point(lon, lat)`; antimeridian-crossing bounding
   boxes use a wrapped longitude-distance query.

## Maturity Review

Architecture maturity: `3/5`.

- Strong: small packages, clear dependency injection, explicit models,
  OpenAPI contract, unit tests around validation/ranking/routing, functional
  Docker e2e coverage, metrics and tracing hooks.
- Weak: production operations are not yet modeled, database-sensitive paths are
  still missing repository-level integration tests and repeatable SQL plan
  evidence, and API handlers still leak raw internal error strings.
- Main upgrade path: add repository-level integration tests and checked-in SQL
  plan fixtures for Postgres, pgvector, migrations, and spatial queries.

Performance maturity: `2/5`.

- Strong: compact response shape, hard result caps, bounded category fan-out,
  concurrent category FTS/vector lookup, and index-aware spatial query intent.
- Weak: the 100M places / 100 ms target from `api/openapi.yaml` is not proven by
  benchmark or load-test artifacts, request latency includes a remote embedding
  call, and core SQL plans are not checked in as repeatable evidence.
- Main upgrade path: add reproducible `EXPLAIN (ANALYZE, BUFFERS)` fixtures and
  load tests for the critical search shapes before tuning code paths.

## Known P0/P1 Risks

- P1: public place identity is `uuid`; internally it maps to `places.fsq_place_id`.
  Keep this translation at repository/service boundaries and do not expose
  `fsq_place_id` in JSON.
- P1: `config/prod.yaml` contains local development defaults. Production must
  rely on environment variables or a separate deployment secret source.
- P1: API handlers currently return raw internal error strings. Public errors
  should be stable and non-leaky; log internal details separately.
- P1: there is no graceful HTTP shutdown or signal handling in `main`.
- P1: migrations are idempotent but not versioned/rollbackable and include
  destructive shape changes. Treat them as development migrations until a
  migration runner and policy exist.

## Performance Notes

- The target is one request, no pagination, max 128 places. Preserve these caps
  unless the OpenAPI contract changes intentionally.
- `places` search uses PostgreSQL `point(lon, lat)` distance. That distance is
  in degrees, not meters. It is acceptable for local ordering prototypes but is
  not true geodesic distance, especially at high latitudes.
- `distance_meters` builds a square bounding box. Near poles or antimeridian
  crossings, longitude can expand to `[-180, 180]`, which is correct as a safe
  fallback but can produce much wider scans.
- Antimeridian-crossing place searches do not use the normal GiST/KNN
  `point(lon, lat)` ordering. They use `search_bbox_antimeridian.sql`, which
  filters and orders by wrapped longitude delta so false neighbors near the
  zero meridian are excluded.
- `categories/search_vector.sql` computes score for all categories then ranks.
  If category count grows, prefer an index-friendly shape:
  `ORDER BY embedding <=> $1::vector LIMIT $2`.
- Every change touching spatial SQL must be validated with
  `EXPLAIN (ANALYZE, BUFFERS)` on representative data sizes and at least these
  locations: dense city, sparse rural, coastline/island, high latitude, and near
  the antimeridian.
- Add p50/p95/p99 latency tracking before claiming progress on the 100 ms goal.
- Consider an embedding cache keyed by normalized query text if repeated user
  intents are common. Do not add it without hit-rate metrics.
- Keep SQL result columns compact; map rendering only needs identifiers, label,
  and coordinates on the hot path.

## Contract Rules

- `api/openapi.yaml` is the API source of truth.
- If a handler, response model, error code, or query parameter changes, update
  OpenAPI and HTTP tests in the same change.
- Do not expose fields in JSON that are not represented in OpenAPI unless the
  contract update is part of the same change.
- Preserve query validation limits:
  - category query length: 1..512 runes
  - category limit: 1..100
  - place limit: 1..128
  - place search category fan-out: max 3 category IDs
  - place uuid length: 1..128 runes
  - `distance_meters`: positive integer, default 5000

## Testing And Verification

Baseline commands from this review:

```sh
go test ./...
go test -race ./...
go vet ./...
go test -bench=. -benchmem ./...
```

Functional Docker e2e command:

```sh
just test-e2e
```

`just test-e2e` uses `build/docker-compose.e2e.yml`, creates/uses the isolated
`fsqr_test` database, runs SQL migrations through the `migrate` service, waits
for TEI/API readiness, and executes:

```sh
go test -tags=e2e -count=1 -v ./tests
```

The e2e source of truth is `docs/test-cases/cases.md`. Each case must be a
top-level `TestE2E_*` function in the corresponding grouped file under
`tests/`, and each case must clean/load fixtures independently.

Observed on 2026-06-02:

- `go test ./...` passed.
- `go test -race ./...` passed.
- `go vet ./...` passed.
- `go test -bench=. -benchmem ./...` passed but found no actual benchmarks.
- `just test-e2e` passed with all 26 functional cases.

Before merging performance-sensitive changes:

```sh
just postgres-up
just postgres-migrate
just places-import-smoke
go test ./...
```

For database-sensitive changes, add or update integration coverage instead of
only testing pure Go normalization functions.

## Coding Rules

- Prefer small interfaces at package boundaries, as the project already does.
- Keep HTTP parsing/serialization in `internal/http`; keep business rules in
  `internal/service`; keep SQL in repository packages.
- Keep direct children under `internal/service` limited to `search`, `category`,
  and `place`. Do not add Go files directly under `internal/service`.
- Inside `internal/service/search`, preserve focused helper subpackages for
  implementations such as `normalizer`, `ranker`, and `scorer`.
- Keep collection behavior for `search.Document` in `internal/models/search`
  close to the document model, not as standalone service helper functions or
  service-local mappers.
- Do not let repository DTOs leak into API models.
- Do not add global mutable state for search/ranking.
- Propagate request contexts through embedding and DB calls.
- Keep logs structured with zap. Do not log API keys or full embedding vectors.
- Keep OpenTelemetry spans useful but avoid high-cardinality attributes beyond
  bounded request metadata.
- Use `rg` for searching and `go test ./...` as the first correctness gate.
- Keep functional e2e tests grouped by the sections in
  `docs/test-cases/cases.md`: validation, categories, search places,
  bbox/distance, geographic intent, and place details.

## Suggested Next Work

1. Add repository-level Postgres integration tests for category vector SQL,
   place KNN SQL, and antimeridian spatial SQL.
2. Add repeatable benchmark/load-test fixtures for the documented 100M/100 ms
   design target.
3. Add graceful shutdown and production-safe error responses.
