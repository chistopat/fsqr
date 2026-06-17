#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
env_file="${KAILAS_ROOT_ENV_FILE:-${FSQR_ROOT_ENV_FILE:-$repo_root/.env}}"

log() {
    printf '[hoppify-s3] %s\n' "$*" >&2
}

env_quote() {
    local value="$1"
    value="${value//\\/\\\\}"
    value="${value//\"/\\\"}"
    value="${value//\$/\\$}"
    value="${value//\`/\\\`}"
    printf '"%s"' "$value"
}

set_env_var() {
    local name="$1"
    local value="$2"
    local quoted
    local tmp

    quoted="$(env_quote "$value")"
    tmp="$(mktemp)"
    if [[ -f "$env_file" ]] && grep -qE "^${name}=" "$env_file"; then
        while IFS= read -r line || [[ -n "$line" ]]; do
            if [[ "$line" == "$name="* ]]; then
                printf '%s=%s\n' "$name" "$quoted" >> "$tmp"
            else
                printf '%s\n' "$line" >> "$tmp"
            fi
        done < "$env_file"
    else
        if [[ -f "$env_file" ]]; then
            cat "$env_file" > "$tmp"
            if [[ -s "$tmp" ]] && [[ "$(tail -c 1 "$tmp")" != "" ]]; then
                printf '\n' >> "$tmp"
            fi
        fi
        printf '%s=%s\n' "$name" "$quoted" >> "$tmp"
    fi

    mv "$tmp" "$env_file"
    chmod 600 "$env_file"
}

slugify() {
    printf '%s' "$1" \
        | tr '[:upper:]' '[:lower:]' \
        | sed -E 's/[^a-z0-9-]+/-/g; s/^-+//; s/-+$//'
}

default_bucket() {
    local source="${FSQR_DNS_ZONE:-${FSQR_DOMAIN:-fsqr-prod}}"
    local slug
    local bucket

    slug="$(slugify "$source")"
    if [[ -z "$slug" ]]; then
        slug="fsqr-prod"
    fi

    bucket="hoppify-${slug}"
    bucket="${bucket:0:63}"
    bucket="${bucket%-}"
    printf '%s\n' "$bucket"
}

require_s3_credentials() {
    local missing=()

    if [[ -z "${HOPPIFY_S3_ACCESS_KEY_ID:-}" ]]; then
        missing+=("HOPPIFY_S3_ACCESS_KEY_ID")
    fi
    if [[ -z "${HOPPIFY_S3_SECRET_ACCESS_KEY:-}" ]]; then
        missing+=("HOPPIFY_S3_SECRET_ACCESS_KEY")
    fi

    if (( ${#missing[@]} == 0 )); then
        return
    fi

    cat >&2 <<EOF
Missing Hetzner Object Storage credentials: ${missing[*]}

Hetzner currently exposes Bucket/Object operations through the S3 API, but S3
credential generation is still done in Hetzner Console, not through HCLOUD_TOKEN.
Create one key pair in the project once, export the variables above, then rerun:

  just cloud

The script will store those values in $env_file, create/check the bucket, and
bootstrap will propagate them to /opt/fsqr/.env.
EOF
    exit 1
}

pre_hoppify_s3_bucket="${HOPPIFY_S3_BUCKET:-}"
pre_hoppify_s3_region="${HOPPIFY_S3_REGION:-}"
pre_hoppify_s3_endpoint_url="${HOPPIFY_S3_ENDPOINT_URL:-}"
pre_hoppify_s3_access_key_id="${HOPPIFY_S3_ACCESS_KEY_ID:-}"
pre_hoppify_s3_secret_access_key="${HOPPIFY_S3_SECRET_ACCESS_KEY:-}"
pre_hoppify_s3_session_token="${HOPPIFY_S3_SESSION_TOKEN:-}"
pre_hoppify_s3_force_path_style="${HOPPIFY_S3_FORCE_PATH_STYLE:-}"
pre_hoppify_s3_bucket_public_read="${HOPPIFY_S3_BUCKET_PUBLIC_READ:-}"

if [[ -f "$env_file" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "$env_file"
    set +a
else
    touch "$env_file"
    chmod 600 "$env_file"
fi

HOPPIFY_S3_BUCKET="${pre_hoppify_s3_bucket:-${HOPPIFY_S3_BUCKET:-}}"
HOPPIFY_S3_REGION="${pre_hoppify_s3_region:-${HOPPIFY_S3_REGION:-}}"
HOPPIFY_S3_ENDPOINT_URL="${pre_hoppify_s3_endpoint_url:-${HOPPIFY_S3_ENDPOINT_URL:-}}"
HOPPIFY_S3_ACCESS_KEY_ID="${pre_hoppify_s3_access_key_id:-${HOPPIFY_S3_ACCESS_KEY_ID:-}}"
HOPPIFY_S3_SECRET_ACCESS_KEY="${pre_hoppify_s3_secret_access_key:-${HOPPIFY_S3_SECRET_ACCESS_KEY:-}}"
HOPPIFY_S3_SESSION_TOKEN="${pre_hoppify_s3_session_token:-${HOPPIFY_S3_SESSION_TOKEN:-}}"
HOPPIFY_S3_FORCE_PATH_STYLE="${pre_hoppify_s3_force_path_style:-${HOPPIFY_S3_FORCE_PATH_STYLE:-}}"
HOPPIFY_S3_BUCKET_PUBLIC_READ="${pre_hoppify_s3_bucket_public_read:-${HOPPIFY_S3_BUCKET_PUBLIC_READ:-}}"

hoppify_s3_region="${HOPPIFY_S3_REGION:-${TF_VAR_location:-${HCLOUD_LOCATION:-hel1}}}"
hoppify_s3_endpoint="${HOPPIFY_S3_ENDPOINT_URL:-https://${hoppify_s3_region}.your-objectstorage.com}"
hoppify_s3_bucket="${HOPPIFY_S3_BUCKET:-$(default_bucket)}"
hoppify_s3_force_path_style="${HOPPIFY_S3_FORCE_PATH_STYLE:-false}"
hoppify_s3_bucket_public_read="${HOPPIFY_S3_BUCKET_PUBLIC_READ:-true}"

set_env_var HOPPIFY_S3_BUCKET "$hoppify_s3_bucket"
set_env_var HOPPIFY_S3_REGION "$hoppify_s3_region"
set_env_var HOPPIFY_S3_ENDPOINT_URL "$hoppify_s3_endpoint"
set_env_var HOPPIFY_S3_FORCE_PATH_STYLE "$hoppify_s3_force_path_style"
set_env_var HOPPIFY_S3_BUCKET_PUBLIC_READ "$hoppify_s3_bucket_public_read"

if [[ -n "${HOPPIFY_S3_ACCESS_KEY_ID:-}" ]]; then
    set_env_var HOPPIFY_S3_ACCESS_KEY_ID "$HOPPIFY_S3_ACCESS_KEY_ID"
fi
if [[ -n "${HOPPIFY_S3_SECRET_ACCESS_KEY:-}" ]]; then
    set_env_var HOPPIFY_S3_SECRET_ACCESS_KEY "$HOPPIFY_S3_SECRET_ACCESS_KEY"
fi
if [[ -n "${HOPPIFY_S3_SESSION_TOKEN:-}" ]]; then
    set_env_var HOPPIFY_S3_SESSION_TOKEN "$HOPPIFY_S3_SESSION_TOKEN"
fi

export HOPPIFY_S3_BUCKET="$hoppify_s3_bucket"
export HOPPIFY_S3_REGION="$hoppify_s3_region"
export HOPPIFY_S3_ENDPOINT_URL="$hoppify_s3_endpoint"
export HOPPIFY_S3_ACCESS_KEY_ID
export HOPPIFY_S3_SECRET_ACCESS_KEY
export HOPPIFY_S3_SESSION_TOKEN
export HOPPIFY_S3_FORCE_PATH_STYLE="$hoppify_s3_force_path_style"
export HOPPIFY_S3_BUCKET_PUBLIC_READ="$hoppify_s3_bucket_public_read"

require_s3_credentials

log "ensuring public bucket $HOPPIFY_S3_BUCKET at $HOPPIFY_S3_ENDPOINT_URL"
(
    cd "$repo_root/apps/hoppify"
    GOWORK=off go run ./cmd/hoppify-s3-ensure
)
