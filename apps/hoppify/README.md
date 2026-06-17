# Hoppify

Hoppify is a single-container app in the Kailas stack. The backend serves API routes and static frontend assets from the same process, following the same deployment model as `apps/fsqr`.

Current production behavior:

- `GET /` serves a placeholder page.
- `GET /assets/*` serves static frontend assets.
- `GET /live` returns a JSON liveness response.
- `GET /metrics` exposes a minimal Prometheus text endpoint.
- `GET /swagger.json` and `GET /swagger` serve OpenAPI documentation.
- `POST /api/v1/captures` accepts 1..10 JPEG, PNG, or WebP images as multipart `files`, converts them
  to JPEG quality 95, stores them in S3-compatible object storage, and records metadata in Postgres.

Required runtime dependencies:

- A dedicated Postgres role and database, defaulting to `hoppify:hoppify@hoppify` locally and
  `hoppify:hoppify@hoppify_test` in e2e. Hoppify must not share the fsqr application database or
  application credentials.
- S3-compatible object storage, configured through the YAML files in `config/` with optional
  `HOPPIFY_*` env overrides.
- Prometheus metrics on the configured metrics address, defaulting to `127.0.0.1:3001` locally.

Configuration is loaded from `config/{local,dev,e2e,prod}.yaml`, selected by `HOPPIFY_ENV`, or from
`HOPPIFY_CONFIG_FILE` when set.

## Development

```sh
go test ./...
go run ./cmd/hoppify
```

The local Compose stack exposes Hoppify on `HOPPIFY_HOST_PORT`, defaulting to `127.0.0.1:3100`, and starts
MinIO with bucket `hoppify` for capture uploads.

Apply Hoppify migrations with:

```sh
just postgres-migrate
```

Run Hoppify functional tests against the Compose stack with:

```sh
just test-hoppify-e2e
```
