#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
deployment_dir="$repo_root/deployment"

app_dir="${FSQR_DEPLOY_DIR:-/opt/fsqr}"
ssh_user="${FSQR_DEPLOY_USER:-deploy}"
ssh_key="${FSQR_DEPLOY_SSH_KEY:-$repo_root/.ssh/fsqr_hcloud_ed25519}"
env_file="${FSQR_ROOT_ENV_FILE:-$repo_root/.env}"
compose_file="$repo_root/build/docker-compose.prod.yml"
caddyfile="$deployment_dir/Caddyfile.prod"
observability_dir="$repo_root/observability"
remote_host="${FSQR_DEPLOY_HOST:-}"

log() {
    printf '[bootstrap] %s\n' "$*" >&2
}

require_file() {
    local path="$1"
    if [[ ! -f "$path" ]]; then
        echo "required file is missing: $path" >&2
        exit 1
    fi
}

resolve_host() {
    if [[ -n "$remote_host" ]]; then
        printf '%s\n' "$remote_host"
        return
    fi

    if ! command -v tofu >/dev/null 2>&1; then
        echo "tofu is required when FSQR_DEPLOY_HOST is not set" >&2
        exit 1
    fi

    local output
    if ! output="$(tofu -chdir="$deployment_dir" output -raw ipv4_address 2>/dev/null)" || [[ -z "$output" ]]; then
        echo "could not resolve server IPv4; set FSQR_DEPLOY_HOST or run tofu apply first" >&2
        exit 1
    fi
    printf '%s\n' "$output"
}

yaml_quote() {
    local value="$1"
    value="${value//\'/\'\'}"
    printf "'%s'" "$value"
}

load_root_env() {
    require_file "$env_file"

    set -a
    # shellcheck disable=SC1090
    source "$env_file"
    set +a
}

require_env() {
    local name="$1"
    if [[ -z "${!name:-}" ]]; then
        echo "$name must be set in $env_file" >&2
        exit 1
    fi
}

generate_config_file() {
    local path="$1"
    local postgres_db="${POSTGRES_DB:-fsqr}"
    local postgres_user="${POSTGRES_USER:-fsqr}"
    local embeddings_api_key="${FSQR_EMBEDDINGS_API_KEY:-tei-local}"
    local embeddings_model="${TEI_SERVED_MODEL_NAME:-intfloat/multilingual-e5-small}"
    local database_dsn

    database_dsn="host=postgres port=5432 dbname=$postgres_db user=$postgres_user password=$POSTGRES_PASSWORD sslmode=disable"

    cat > "$path" <<EOF_CONFIG
app:
  name: fsqr
  env: prod

http:
  addr: ":3000"

logger:
  level: info
  encoding: json
  development: false

database:
  dsn: $(yaml_quote "$database_dsn")
  max_open_conns: 8
  max_idle_conns: 4
  conn_max_lifetime: 30m
  conn_max_idle_time: 5m
  connect_timeout: 5s

embeddings:
  base_url: "http://tei/v1"
  api_key: $(yaml_quote "$embeddings_api_key")
  model: $(yaml_quote "$embeddings_model")
  timeout: 10s

observability:
  service_name: fsqr
  metrics:
    addr: ":3001"
    path: "/metrics"
  tracing:
    enabled: false
    otlp_endpoint_url: ""
    otlp_insecure: true
EOF_CONFIG
}

write_env_var() {
    local path="$1"
    local name="$2"
    local value="$3"

    if [[ "$value" == *$'\n'* ]]; then
        echo "$name contains a newline, which is not supported in Docker Compose env files" >&2
        exit 1
    fi

    value="${value//\\/\\\\}"
    value="${value//\"/\\\"}"
    value="${value//\$/\\$}"
    value="${value//\`/\\\`}"

    printf '%s=\"%s\"\n' "$name" "$value" >> "$path"
}

generate_compose_env_file() {
    local path="$1"

    : > "$path"
    write_env_var "$path" FSQR_IMAGE "${FSQR_IMAGE:-ghcr.io/chistopat/fsqr:latest}"
    write_env_var "$path" FSQR_DOMAIN "$FSQR_DOMAIN"
    write_env_var "$path" GRAFANA_DOMAIN "${GRAFANA_DOMAIN:-grafana.$FSQR_DOMAIN}"
    write_env_var "$path" POSTGRES_DB "${POSTGRES_DB:-fsqr}"
    write_env_var "$path" POSTGRES_USER "${POSTGRES_USER:-fsqr}"
    write_env_var "$path" POSTGRES_PASSWORD "$POSTGRES_PASSWORD"
    write_env_var "$path" TEI_IMAGE "${TEI_IMAGE:-ghcr.io/huggingface/text-embeddings-inference:cpu-1.9}"
    write_env_var "$path" TEI_PLATFORM "${TEI_PLATFORM:-linux/amd64}"
    write_env_var "$path" TEI_MODEL_ID "${TEI_MODEL_ID:-intfloat/multilingual-e5-small}"
    write_env_var "$path" TEI_MODEL_REVISION "${TEI_MODEL_REVISION:-fd1525a9fd15316a2d503bf26ab031a61d056e98}"
    write_env_var "$path" TEI_SERVED_MODEL_NAME "${TEI_SERVED_MODEL_NAME:-intfloat/multilingual-e5-small}"
    write_env_var "$path" HF_TOKEN "${HF_TOKEN:-}"
    write_env_var "$path" PROMETHEUS_IMAGE "${PROMETHEUS_IMAGE:-prom/prometheus:latest}"
    write_env_var "$path" GRAFANA_IMAGE "${GRAFANA_IMAGE:-grafana/grafana:latest}"
    write_env_var "$path" GRAFANA_ADMIN_USER "${GRAFANA_ADMIN_USER:-admin}"
    write_env_var "$path" GRAFANA_ADMIN_PASSWORD "$GRAFANA_ADMIN_PASSWORD"
    write_env_var "$path" GOOSE_RUNNER_IMAGE "${GOOSE_RUNNER_IMAGE:-ghcr.io/chistopat/fsqr-goose:v3.27.1}"
    write_env_var "$path" GOOSE_VERSION "${GOOSE_VERSION:-v3.27.1}"
    write_env_var "$path" WATCHTOWER_INTERVAL "${WATCHTOWER_INTERVAL:-300}"
    write_env_var "$path" GHCR_USERNAME "${GHCR_USERNAME:-}"
    write_env_var "$path" GHCR_TOKEN "${GHCR_TOKEN:-}"
}

wait_for_ssh() {
    local target="$1"
    local attempt
    for attempt in {1..60}; do
        if ssh "${ssh_opts[@]}" "$target" 'true' >/dev/null 2>&1; then
            return
        fi
        sleep 5
    done

    echo "ssh did not become ready for $target" >&2
    exit 1
}

require_file "$compose_file"
require_file "$caddyfile"
require_file "$ssh_key"
load_root_env
require_env POSTGRES_PASSWORD
require_env FSQR_DOMAIN
require_env GRAFANA_ADMIN_PASSWORD

host="$(resolve_host)"
target="$ssh_user@$host"
ssh_opts=(-i "$ssh_key" -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new)
scp_opts=(-i "$ssh_key" -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new)

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

mkdir -p "$tmpdir/migrations"
cp "$compose_file" "$tmpdir/compose.yml"
cp "$caddyfile" "$tmpdir/Caddyfile"
cp -R "$observability_dir" "$tmpdir/observability"
generate_compose_env_file "$tmpdir/.env"
generate_config_file "$tmpdir/config.yaml"
cp "$repo_root"/migrations/*.sql "$tmpdir/migrations/"
chmod 600 "$tmpdir/.env"
chmod 644 "$tmpdir/config.yaml"

log "waiting for ssh: $target"
wait_for_ssh "$target"

log "waiting for cloud-init to finish"
ssh "${ssh_opts[@]}" "$target" 'sudo cloud-init status --wait || true'

remote_tmp="/tmp/fsqr-bootstrap-$$"
log "copying compose bundle to $target:$app_dir"
ssh "${ssh_opts[@]}" "$target" "rm -rf '$remote_tmp' && mkdir -p '$remote_tmp'"
scp "${scp_opts[@]}" -r "$tmpdir"/. "$target:$remote_tmp/"

log "starting docker compose stack"
ssh "${ssh_opts[@]}" "$target" APP_DIR="$app_dir" REMOTE_TMP="$remote_tmp" 'bash -s' <<'EOF_REMOTE'
set -euo pipefail

sudo mkdir -p "$APP_DIR"
sudo cp -R "$REMOTE_TMP"/. "$APP_DIR"/
sudo chown -R "$USER:$USER" "$APP_DIR"
rm -rf "$REMOTE_TMP"

for attempt in {1..60}; do
    if docker info >/dev/null 2>&1; then
        break
    fi
    if [[ "$attempt" == "60" ]]; then
        echo "docker did not become ready" >&2
        exit 1
    fi
    sleep 2
done

cd "$APP_DIR"
chmod 600 .env
chmod 644 config.yaml

set -a
# The generated .env quotes values, so source it instead of parsing raw lines.
# Otherwise GHCR_USERNAME="" is read as a non-empty literal and triggers a bad login.
source .env
set +a

ghcr_user="${GHCR_USERNAME:-}"
ghcr_token="${GHCR_TOKEN:-}"
if [[ -n "$ghcr_user" && -n "$ghcr_token" ]]; then
    printf '%s' "$ghcr_token" | docker login ghcr.io -u "$ghcr_user" --password-stdin >/dev/null
fi

docker compose pull fsqr caddy postgres tei prometheus grafana watchtower
docker compose up -d postgres tei
docker compose up -d --remove-orphans fsqr prometheus grafana caddy watchtower
EOF_REMOTE

log "bootstrap completed: ssh $target, app dir $app_dir"
