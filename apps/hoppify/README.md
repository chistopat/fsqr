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
- `POST /api/v1/detect` accepts a stored capture UUID and returns Ultralytics-style object detections.
- `POST /api/v1/beer-labels/identify` accepts a stored capture or crop UUID, asks the configured OpenAI
  vision model for structured beer label identification without web search, and caches the result in
  Postgres by UUID and prompt version.
- `POST /api/v2/beer-labels/identify` uses the same request body, allows OpenAI hosted web search for
  verification, and may return web sources plus an Untappd direct-match or search recommendation.

Required runtime dependencies:

- A dedicated Postgres role and database, defaulting to `hoppify:hoppify@hoppify` locally and
  `hoppify:hoppify@hoppify_test` in e2e. Hoppify must not share the fsqr application database or
  application credentials.
- S3-compatible object storage, configured through the YAML files in `config/` with optional
  `HOPPIFY_*` env overrides.
- ONNX Runtime and the SKU-110K YOLO11s 640 ONNX model. Production and e2e Docker images download
  `weights/sku110k-yolo11-s640.onnx` during image build, verify its SHA256 checksum, and copy it to
  `/app/models/sku110k-yolo11-s640.onnx` with `libonnxruntime.so.1.22.0` under `/app/lib`.
- OpenAI API access for beer label recognition. Configure `HOPPIFY_BEER_LABEL_OPENAI_API_KEY` or
  `OPENAI_API_KEY`; without a key, the beer label endpoint returns `model_unavailable`.
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

The Hoppify e2e suite uploads `tests/fixtures/detect-shelf.jpg`, calls `/api/v1/detect` by UUID, and
asserts real model bounding boxes. The same e2e suite runs in CI after building the Docker image that
contains the model and ONNX Runtime.
