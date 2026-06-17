# Hoppify

Hoppify is a single-container app in the Kailas stack. The backend serves API routes and static frontend assets from the same process, following the same deployment model as `apps/fsqr`.

Current production behavior:

- `GET /` serves a placeholder page.
- `GET /assets/*` serves static frontend assets.
- `GET /live` returns a JSON liveness response.
- `GET /metrics` exposes a minimal Prometheus text endpoint.

## Development

```sh
go test ./...
go run ./cmd/hoppify
```

The local Compose stack exposes Hoppify on `HOPPIFY_HOST_PORT`, defaulting to `127.0.0.1:3100`.
