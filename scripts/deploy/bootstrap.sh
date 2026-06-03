#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
deployment_dir="$repo_root/deployment"

app_dir="${FSQR_DEPLOY_DIR:-/opt/fsqr}"
ssh_user="${FSQR_DEPLOY_USER:-deploy}"
ssh_key="${FSQR_DEPLOY_SSH_KEY:-$repo_root/.ssh/fsqr_hcloud_ed25519}"
env_file="${FSQR_DEPLOY_ENV_FILE:-$deployment_dir/.env.deploy}"
compose_file="$deployment_dir/compose.prod.yml"
caddyfile="$deployment_dir/Caddyfile.prod"
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

generate_env_file() {
    local postgres_password
    postgres_password="$(openssl rand -hex 32)"

    umask 077
    cat > "$env_file" <<EOF_ENV
FSQR_IMAGE=${FSQR_IMAGE:-ghcr.io/chistopat/fsqr:prod}
FSQR_DOMAIN=${FSQR_DOMAIN:-:80}

POSTGRES_DB=${POSTGRES_DB:-fsqr}
POSTGRES_USER=${POSTGRES_USER:-fsqr}
POSTGRES_PASSWORD=$postgres_password

FSQR_EMBEDDINGS_API_KEY=${FSQR_EMBEDDINGS_API_KEY:-tei-local}
TEI_IMAGE=${TEI_IMAGE:-ghcr.io/huggingface/text-embeddings-inference:cpu-1.9}
TEI_PLATFORM=${TEI_PLATFORM:-linux/amd64}
TEI_MODEL_ID=${TEI_MODEL_ID:-intfloat/multilingual-e5-small}
TEI_MODEL_REVISION=${TEI_MODEL_REVISION:-fd1525a9fd15316a2d503bf26ab031a61d056e98}
TEI_SERVED_MODEL_NAME=${TEI_SERVED_MODEL_NAME:-intfloat/multilingual-e5-small}
HF_TOKEN=${HF_TOKEN:-}

WATCHTOWER_INTERVAL=${WATCHTOWER_INTERVAL:-300}
GHCR_USERNAME=${GHCR_USERNAME:-}
GHCR_TOKEN=${GHCR_TOKEN:-}
EOF_ENV
}

ensure_env_file() {
    if [[ -f "$env_file" ]]; then
        return
    fi

    if ! command -v openssl >/dev/null 2>&1; then
        echo "openssl is required to generate $env_file" >&2
        exit 1
    fi

    log "creating ignored deploy env: $env_file"
    generate_env_file
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
ensure_env_file
require_file "$env_file"

host="$(resolve_host)"
target="$ssh_user@$host"
ssh_opts=(-i "$ssh_key" -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new)
scp_opts=(-i "$ssh_key" -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new)

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

mkdir -p "$tmpdir/migrations"
cp "$compose_file" "$tmpdir/compose.yml"
cp "$caddyfile" "$tmpdir/Caddyfile"
cp "$env_file" "$tmpdir/.env"
cp "$repo_root"/migrations/*.sql "$tmpdir/migrations/"
chmod 600 "$tmpdir/.env"

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

ghcr_user="$(sed -n 's/^GHCR_USERNAME=//p' .env | tail -n 1)"
ghcr_token="$(sed -n 's/^GHCR_TOKEN=//p' .env | tail -n 1)"
if [[ -n "$ghcr_user" && -n "$ghcr_token" ]]; then
    printf '%s' "$ghcr_token" | docker login ghcr.io -u "$ghcr_user" --password-stdin >/dev/null
fi

docker compose pull
docker compose up -d postgres tei
docker compose run --rm migrate
docker compose up -d --remove-orphans fsqr caddy watchtower
EOF_REMOTE

log "bootstrap completed: ssh $target, app dir $app_dir"
