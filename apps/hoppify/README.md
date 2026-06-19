# Hoppify

Hoppify is a single-container app in the Kailas stack. The backend serves API routes and static frontend assets from the same process, following the same deployment model as `apps/fsqr`.

Current production behavior:

- `GET /` serves the React captures gallery.
- `GET /assets/*` serves static frontend assets.
- `GET /live` returns a JSON liveness response.
- `GET /metrics` exposes a minimal Prometheus text endpoint.
- `GET /swagger.json` and `GET /swagger` serve OpenAPI documentation.
- `GET /api/v1/captures` returns top-level image captures ordered from newest to oldest with
  `limit`/`offset` pagination and browser-ready `imageUrl` values.
- `GET /api/v1/captures/{uuid}/image` streams the stored JPEG for a capture or crop UUID.
- `POST /api/v1/captures` accepts 1..10 JPEG, PNG, or WebP images as multipart `files`, stores JPEG
  uploads byte-for-byte without recompression, converts PNG/WebP uploads to JPEG quality 95, stores
  them in S3-compatible object storage, and records metadata in Postgres. Responses include `uuid`,
  `uri`, and `url`.
- `POST /api/v1/detect` accepts exactly one source: JSON `uuid`, JSON `url`/`uri` with an `s3://...`
  object URI, or a multipart `file`, runs all configured detector models, and returns one merged
  Ultralytics-style detection pool.
- `POST /api/v1/crops` creates JPEG crops and returns each crop `uuid`, `uri`, and `url`.
- `POST /api/v1/beer-labels/identify` accepts JSON `uuid`, `url`, or `uri`, uses Gemini 2.5 Flash-Lite
  with the v3 vision-only prompt, and caches by UUID and v3 prompt version when the source maps to a
  stored capture or crop.

Required runtime dependencies:

- A dedicated Postgres role and database, defaulting to `hoppify:hoppify@hoppify` locally and
  `hoppify:hoppify@hoppify_test` in e2e. Hoppify must not share the fsqr application database or
  application credentials.
- S3-compatible object storage, configured through the YAML files in `config/` with optional
  `HOPPIFY_*` env overrides.
- ONNX Runtime, the SKU-110K YOLO11s 640 ONNX model, and the Hoppify YOLO11n 640 ONNX model.
  Production and e2e Docker images verify both model SHA256 checksums and copy them to
  `/app/models/sku110k-yolo11-s640.onnx` and `/app/models/hoppify-yolo11-640n.onnx` with
  `libonnxruntime.so.1.22.0` under `/app/lib`.
- Gemini API access for beer label recognition. Configure `HOPPIFY_BEER_LABEL_GEMINI_API_KEY`,
  `GEMINI_API_KEY`, or `GOOGLE_API_KEY`; without a key, the beer label endpoint returns
  `model_unavailable`.
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

## Untappd beer catalog

The local `data/beers.parquet` file is intentionally ignored by git. It contains Untappd catalog rows with
`url`, `untappd_id`, `slog`, `brewery_prefix`, and `last_modified_at`. Migration `006_create_untappd_beers.sql`
creates the Postgres target table and search indexes for importing this file.

Import the parquet fields like this:

- `untappd_id` -> `untappd_id`
- `url` -> `url`
- `slog` -> `untappd_slug`
- `brewery_prefix` -> `brewery_prefix`
- `last_modified_at` -> `last_modified_at`
- `search_text`: lower-case searchable text made from `slog`, replacing non-alphanumeric characters with spaces
  and collapsing repeated spaces.

For direct Untappd extraction from OCR or model output, parse a full `https://untappd.com/b/.../<id>` URL or a
trailing numeric Untappd id and look up by `untappd_id` or `url`. For OCR descriptions without a URL, combine
full-text search with trigram similarity:

```sql
WITH query AS (
    SELECT
        websearch_to_tsquery('simple', $1) AS ts_query,
        btrim(regexp_replace(lower($1), '[^[:alnum:]]+', ' ', 'g')) AS normalized_query
)
SELECT
    beer.untappd_id,
    beer.url,
    beer.untappd_slug,
    beer.brewery_prefix,
    ts_rank_cd(to_tsvector('simple', beer.search_text), query.ts_query) AS text_rank,
    similarity(beer.search_text, query.normalized_query) AS fuzzy_rank
FROM untappd_beers beer, query
WHERE to_tsvector('simple', beer.search_text) @@ query.ts_query
    OR beer.search_text % query.normalized_query
ORDER BY
    text_rank DESC,
    fuzzy_rank DESC,
    beer.untappd_id
LIMIT 10;
```

Run Hoppify functional tests against the Compose stack with:

```sh
just test-hoppify-e2e
```

The Hoppify e2e suite uploads `tests/fixtures/detect-shelf.jpg`, calls `/api/v1/detect` by UUID, and
asserts real model bounding boxes. The same e2e suite runs in CI after building the Docker image that
contains the model and ONNX Runtime.
