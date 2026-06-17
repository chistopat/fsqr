set dotenv-load := true

compose := "docker compose -f deployment/compose.local.yml"
e2e_compose := "docker compose -f deployment/compose.e2e.yml"
fsqr_image := "fsqr:local"
hoppify_image := "hoppify:local"
tei_url := "http://127.0.0.1:8080"
postgres_psql := compose + " exec -T postgres psql -v ON_ERROR_STOP=1 -U \"${POSTGRES_USER:-fsqr}\" -d \"${POSTGRES_DB:-fsqr}\""
golangci_lint := "go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.8.0"

default:
    just --list

service-build:
    cd apps/fsqr && GOWORK=off go build -o ../../bin/fsqr ./cmd/fsqr
    cd apps/hoppify && GOWORK=off go build -o ../../bin/hoppify ./cmd/hoppify

lint:
    cd apps/fsqr && GOWORK=off {{golangci_lint}} run ./... --timeout=5m
    cd apps/hoppify && GOWORK=off {{golangci_lint}} run --config ../../.golangci.yml ./... --timeout=5m

lint-fix:
    cd apps/fsqr && GOWORK=off {{golangci_lint}} run --fix ./... --timeout=5m
    cd apps/hoppify && GOWORK=off {{golangci_lint}} run --config ../../.golangci.yml --fix ./... --timeout=5m

pre-commit: service-build lint
    cd apps/fsqr && GOWORK=off go test ./...
    cd apps/fsqr && GOWORK=off go vet ./...
    cd apps/hoppify && GOWORK=off go test ./...
    cd apps/hoppify && GOWORK=off go vet ./...

fsqr-docker-build tag=fsqr_image:
    docker build -f apps/fsqr/build/Dockerfile -t {{tag}} apps/fsqr

hoppify-docker-build tag=hoppify_image:
    docker build -f apps/hoppify/build/Dockerfile -t {{tag}} apps/hoppify

up:
    {{e2e_compose}} up -d --build postgres tei
    just test-postgres-wait
    just test-db-create
    {{e2e_compose}} build fsqr hoppify
    {{e2e_compose}} up -d fsqr hoppify

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
    {{compose}} run --build --rm migrate-fsqr

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
    {{e2e_compose}} run --build --rm migrate-fsqr

test-postgres-psql:
    {{e2e_compose}} exec postgres psql -U "${POSTGRES_USER:-fsqr}" -d "${TEST_POSTGRES_DB:-fsqr_test}"

test-e2e-wait:
    #!/usr/bin/env bash
    set -euo pipefail
    base_url="${BASE_URL:-http://127.0.0.1:3000}"
    hoppify_base_url="${HOPPIFY_BASE_URL:-http://127.0.0.1:3100}"
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
            break
        fi
        if [[ "$attempt" == "60" ]]; then
            echo "fsqr api did not become ready in time" >&2
            exit 1
        fi
        sleep 1
    done

    for attempt in {1..60}; do
        if curl -fsS "$hoppify_base_url/live" >/dev/null 2>&1; then
            exit 0
        fi
        if [[ "$attempt" == "60" ]]; then
            echo "hoppify app did not become ready in time" >&2
            exit 1
        fi
        sleep 1
    done

test-e2e: up test-migrate test-e2e-wait
    cd apps/fsqr && TEST_DATABASE_URL="${TEST_DATABASE_URL:-postgres://${POSTGRES_USER:-fsqr}:${POSTGRES_PASSWORD:-fsqr}@127.0.0.1:5432/${TEST_POSTGRES_DB:-fsqr_test}?sslmode=disable}" BASE_URL="${BASE_URL:-http://127.0.0.1:3000}" GOWORK=off go test -tags=e2e -count=1 -v ./tests

places-migrate: postgres-migrate

places-import parquet_glob='data/hf/release/dt=2026-05-14/places/parquet/*.parquet': places-migrate
    cd etl && uv run python scripts/import_places.py --parquet-glob '{{parquet_glob}}' --truncate --rebuild-indexes --allow-unknown-categories

places-import-smoke limit='1000': places-migrate
    cd etl && uv run python scripts/import_places.py --limit {{limit}} --truncate --rebuild-indexes --allow-unknown-categories

geo-features-migrate: postgres-migrate

geo-features-import release='2026-05-20.0': geo-features-migrate
    cd etl && uv run python scripts/import_overture_divisions.py --release '{{release}}' --truncate

geo-features-import-smoke release='2026-05-20.0' limit='1000': geo-features-migrate
    cd etl && uv run python scripts/import_overture_divisions.py --release '{{release}}' --limit {{limit}} --truncate --no-progress

categories-embed:
    cd etl && uv run python enricher/embed_categories.py

cloud:
    #!/usr/bin/env bash
    set -euo pipefail
    token="${HCLOUD_TOKEN:-${HCLOUD_API_KEY:-}}"
    if [[ -z "$token" ]]; then
        echo "HCLOUD_TOKEN or HCLOUD_API_KEY must be set" >&2
        exit 1
    fi
    if [[ -z "${FSQR_DOMAIN:-}" ]]; then
        echo "FSQR_DOMAIN must be set" >&2
        exit 1
    fi
    if [[ -z "${FSQR_DNS_ZONE:-}" ]]; then
        echo "FSQR_DNS_ZONE must be set" >&2
        exit 1
    fi

    export HCLOUD_TOKEN="$token"
    export TF_VAR_hcloud_token="$token"
    export TF_VAR_app_domain="$FSQR_DOMAIN"
    export TF_VAR_hoppify_domain="${HOPPIFY_DOMAIN:-hoppify.$FSQR_DNS_ZONE}"
    export TF_VAR_grafana_domain="${GRAFANA_DOMAIN:-grafana.$FSQR_DOMAIN}"
    export TF_VAR_dns_zone_name="$FSQR_DNS_ZONE"

    cd deployment
    tofu init -input=false
    tofu apply

bootstrap:
    ./scripts/deploy/bootstrap.sh

migrate:
    #!/usr/bin/env bash
    set -euo pipefail

    app_dir="${KAILAS_DEPLOY_DIR:-${FSQR_DEPLOY_DIR:-/opt/fsqr}}"
    ssh_user="${KAILAS_DEPLOY_USER:-${FSQR_DEPLOY_USER:-deploy}}"
    ssh_key="${KAILAS_DEPLOY_SSH_KEY:-${FSQR_DEPLOY_SSH_KEY:-.ssh/fsqr_hcloud_ed25519}}"
    remote_host="${KAILAS_DEPLOY_HOST:-${FSQR_DEPLOY_HOST:-}}"

    if [[ ! -f "$ssh_key" ]]; then
        echo "required file is missing: $ssh_key" >&2
        exit 1
    fi

    if [[ -z "$remote_host" ]]; then
        if ! command -v tofu >/dev/null 2>&1; then
            echo "tofu is required when KAILAS_DEPLOY_HOST/FSQR_DEPLOY_HOST is not set" >&2
            exit 1
        fi
        remote_host="$(tofu -chdir=deployment output -raw ipv4_address)"
    fi

    target="$ssh_user@$remote_host"
    ssh_opts=(-i "$ssh_key" -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new)
    rsync_ssh="ssh -i $ssh_key -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new"

    ssh "${ssh_opts[@]}" "$target" APP_DIR="$app_dir" 'bash -s' <<'EOF'
    set -euo pipefail
    if [[ ! -f "$APP_DIR/.env" ]]; then
        echo ".env is missing in $APP_DIR; run just bootstrap first" >&2
        exit 1
    fi
    mkdir -p "$APP_DIR/deployment" "$APP_DIR/apps/fsqr/migrations"
    EOF

    rsync -az -e "$rsync_ssh" deployment/compose.prod.yml "$target:$app_dir/deployment/compose.prod.yml"
    rsync -az --delete -e "$rsync_ssh" apps/fsqr/migrations/ "$target:$app_dir/apps/fsqr/migrations/"

    ssh "${ssh_opts[@]}" "$target" APP_DIR="$app_dir" 'bash -s' <<'EOF'
    set -euo pipefail
    cd "$APP_DIR"
    docker compose --env-file .env -f deployment/compose.prod.yml run --rm migrate-fsqr
    EOF
