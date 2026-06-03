set dotenv-load := true

compose := "docker compose -f build/docker-compose.yml"
e2e_compose := "docker compose -f build/docker-compose.e2e.yml"
fsqr_image := "fsqr:local"
tei_url := "http://127.0.0.1:8080"
postgres_psql := compose + " exec -T postgres psql -v ON_ERROR_STOP=1 -U \"${POSTGRES_USER:-fsqr}\" -d \"${POSTGRES_DB:-fsqr}\""
golangci_lint := "go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.8.0"

default:
    just --list

service-build:
    go build -o bin/fsqr ./cmd/fsqr

lint:
    {{golangci_lint}} run ./... --timeout=5m

lint-fix:
    {{golangci_lint}} run --fix ./... --timeout=5m

pre-commit: service-build lint
    go test ./...
    go vet ./...

fsqr-docker-build tag=fsqr_image:
    docker build -f build/fsqr.Dockerfile -t {{tag}} .

up:
    {{e2e_compose}} up -d --build postgres tei
    just test-postgres-wait
    just test-db-create
    {{e2e_compose}} build fsqr
    {{e2e_compose}} up -d fsqr

down:
    {{e2e_compose}} down

tei-build:
    {{compose}} build tei

tei-up:
    {{compose}} up -d tei

tei-down:
    {{compose}} down

tei-logs:
    {{compose}} logs -f tei

tei-info:
    curl -s {{tei_url}}/info

tei-embed-test:
    curl -s {{tei_url}}/embed -H 'Content-Type: application/json' -d '{"inputs":"passage: Arts and Entertainment"}'

postgres-up:
    {{compose}} up -d postgres

postgres-down:
    {{compose}} stop postgres

postgres-logs:
    {{compose}} logs -f postgres

postgres-status:
    {{compose}} ps postgres

postgres-wait:
    #!/usr/bin/env bash
    set -euo pipefail
    user="${POSTGRES_USER:-fsqr}"
    for attempt in {1..60}; do
        if {{compose}} exec -T postgres pg_isready -U "$user" -d postgres >/dev/null 2>&1; then
            exit 0
        fi
        sleep 1
    done
    echo "postgres did not become ready in time" >&2
    exit 1

postgres-psql:
    {{compose}} exec postgres psql -U "${POSTGRES_USER:-fsqr}" -d "${POSTGRES_DB:-fsqr}"

postgres-migrate:
    {{postgres_psql}} < migrations/001_create_categories.sql
    {{postgres_psql}} < migrations/002_create_places.sql
    {{postgres_psql}} < migrations/003_create_places_fsq_place_id_index.sql

test-db-create:
    #!/usr/bin/env bash
    set -euo pipefail
    db="${TEST_POSTGRES_DB:-fsqr_test}"
    user="${POSTGRES_USER:-fsqr}"
    {{e2e_compose}} exec -T postgres psql -v ON_ERROR_STOP=1 -U "$user" -d postgres -v db="$db" <<'SQL'
    SELECT format('CREATE DATABASE %I', :'db')
    WHERE NOT EXISTS (
        SELECT 1 FROM pg_database WHERE datname = :'db'
    )
    \gexec
    SQL

test-postgres-wait:
    #!/usr/bin/env bash
    set -euo pipefail
    user="${POSTGRES_USER:-fsqr}"
    for attempt in {1..60}; do
        if {{e2e_compose}} exec -T postgres pg_isready -U "$user" -d postgres >/dev/null 2>&1; then
            exit 0
        fi
        sleep 1
    done
    echo "test postgres did not become ready in time" >&2
    exit 1

test-migrate: test-db-create
    {{e2e_compose}} run --rm migrate

test-postgres-psql:
    {{e2e_compose}} exec postgres psql -U "${POSTGRES_USER:-fsqr}" -d "${TEST_POSTGRES_DB:-fsqr_test}"

test-e2e-wait:
    #!/usr/bin/env bash
    set -euo pipefail
    base_url="${BASE_URL:-http://127.0.0.1:3000}"
    tei_base_url="${TEI_URL:-{{tei_url}}}"

    for attempt in {1..180}; do
        if curl -fsS "$tei_base_url/info" >/dev/null 2>&1; then
            break
        fi
        if [[ "$attempt" == "180" ]]; then
            echo "tei did not become ready in time" >&2
            exit 1
        fi
        sleep 1
    done

    for attempt in {1..60}; do
        if curl -fsS "$base_url/live" >/dev/null 2>&1; then
            exit 0
        fi
        if [[ "$attempt" == "60" ]]; then
            echo "fsqr api did not become ready in time" >&2
            exit 1
        fi
        sleep 1
    done

test-e2e: up test-migrate test-e2e-wait
    TEST_DATABASE_URL="${TEST_DATABASE_URL:-postgres://${POSTGRES_USER:-fsqr}:${POSTGRES_PASSWORD:-fsqr}@127.0.0.1:5432/${TEST_POSTGRES_DB:-fsqr_test}?sslmode=disable}" BASE_URL="${BASE_URL:-http://127.0.0.1:3000}" go test -tags=e2e -count=1 -v ./tests

places-migrate: postgres-migrate

places-import parquet_glob='data/hf/release/dt=2026-05-14/places/parquet/*.parquet': places-migrate
    cd etl && uv run python scripts/import_places.py --parquet-glob '{{parquet_glob}}' --truncate --rebuild-indexes --allow-unknown-categories

places-import-smoke limit='1000': places-migrate
    cd etl && uv run python scripts/import_places.py --limit {{limit}} --truncate --rebuild-indexes --allow-unknown-categories

categories-embed:
    cd etl && uv run python enricher/embed_categories.py

deploy:
    #!/usr/bin/env bash
    set -euo pipefail
    token="${HCLOUD_TOKEN:-${HCLOUD_API_KEY:-}}"
    if [[ -z "$token" ]]; then
        echo "HCLOUD_TOKEN or HCLOUD_API_KEY must be set" >&2
        exit 1
    fi

    export HCLOUD_TOKEN="$token"
    export TF_VAR_hcloud_token="$token"

    cd deployment
    tofu init -input=false
    tofu apply

bootstrap:
    ./scripts/deploy/bootstrap.sh
