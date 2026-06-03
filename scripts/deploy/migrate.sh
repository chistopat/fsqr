#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
deployment_dir="$repo_root/deployment"

app_dir="${FSQR_DEPLOY_DIR:-/opt/fsqr}"
ssh_user="${FSQR_DEPLOY_USER:-deploy}"
ssh_key="${FSQR_DEPLOY_SSH_KEY:-$repo_root/.ssh/fsqr_hcloud_ed25519}"
remote_host="${FSQR_DEPLOY_HOST:-}"

log() {
    printf '[migrate] %s\n' "$*" >&2
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

require_file "$ssh_key"

shopt -s nullglob
migration_files=("$repo_root"/migrations/*.sql)
if [[ "${#migration_files[@]}" -eq 0 ]]; then
    echo "no SQL migrations found under $repo_root/migrations" >&2
    exit 1
fi

host="$(resolve_host)"
target="$ssh_user@$host"
ssh_opts=(-i "$ssh_key" -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new)
scp_opts=(-i "$ssh_key" -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new)

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

mkdir -p "$tmpdir/migrations"
cp "${migration_files[@]}" "$tmpdir/migrations/"

remote_tmp="/tmp/fsqr-migrate-$$"
log "copying migrations to $target:$app_dir"
ssh "${ssh_opts[@]}" "$target" "rm -rf '$remote_tmp' && mkdir -p '$remote_tmp'"
scp "${scp_opts[@]}" -r "$tmpdir"/. "$target:$remote_tmp/"

log "running migrations"
ssh "${ssh_opts[@]}" "$target" APP_DIR="$app_dir" REMOTE_TMP="$remote_tmp" 'bash -s' <<'EOF_REMOTE'
set -euo pipefail

if [[ ! -f "$APP_DIR/compose.yml" ]]; then
    echo "compose.yml is missing in $APP_DIR; run just bootstrap first" >&2
    exit 1
fi
if [[ ! -f "$APP_DIR/.env" ]]; then
    echo ".env is missing in $APP_DIR; run just bootstrap first" >&2
    exit 1
fi

mkdir -p "$APP_DIR/migrations"
cp -R "$REMOTE_TMP"/migrations/. "$APP_DIR/migrations"/
rm -rf "$REMOTE_TMP"

cd "$APP_DIR"
docker compose up -d postgres
docker compose run --rm migrate
EOF_REMOTE

log "migrations completed"
