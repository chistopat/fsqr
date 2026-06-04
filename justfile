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
    {{compose}} run --build --rm migrate

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
    {{e2e_compose}} run --build --rm migrate

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

    app_dir="${FSQR_DEPLOY_DIR:-/opt/fsqr}"
    ssh_user="${FSQR_DEPLOY_USER:-deploy}"
    ssh_key="${FSQR_DEPLOY_SSH_KEY:-.ssh/fsqr_hcloud_ed25519}"
    remote_host="${FSQR_DEPLOY_HOST:-}"

    if [[ ! -f "$ssh_key" ]]; then
        echo "required file is missing: $ssh_key" >&2
        exit 1
    fi

    if [[ -z "$remote_host" ]]; then
        if ! command -v tofu >/dev/null 2>&1; then
            echo "tofu is required when FSQR_DEPLOY_HOST is not set" >&2
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
    mkdir -p "$APP_DIR/migrations"
    EOF

    rsync -az -e "$rsync_ssh" build/docker-compose.prod.yml "$target:$app_dir/compose.yml"
    rsync -az --delete -e "$rsync_ssh" migrations/ "$target:$app_dir/migrations/"

    ssh "${ssh_opts[@]}" "$target" APP_DIR="$app_dir" 'bash -s' <<'EOF'
    set -euo pipefail
    cd "$APP_DIR"
    docker compose run --rm migrate
    EOF
