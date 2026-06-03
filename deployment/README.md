# fsqr Hetzner Deployment

OpenTofu configuration for one Hetzner Cloud VM in Helsinki with Docker CE preinstalled.

## Resources

- Firewall: inbound TCP `22`, `80`, `443` only.
- Static Primary IPv4: `auto_delete = false` so DNS can keep using the same address after VM replacement.
- Server: `cpx42` in `hel1` Helsinki, image `docker-ce`, IPv4 enabled, IPv6 disabled.
- SSH key: uploaded from `../.ssh/fsqr_hcloud_ed25519.pub`.
- Cloud-init: creates non-root `deploy` user, disables password auth and root SSH, verifies Docker Compose.

## Commands

Secrets are read from the repository root `.env`; do not create deployment-local
env files. At minimum, production bootstrap needs:

```env
HCLOUD_API_KEY=...
POSTGRES_PASSWORD=...
```

Optional root `.env` values used by Compose/bootstrap:

```env
FSQR_IMAGE=ghcr.io/chistopat/fsqr:latest
FSQR_DOMAIN=:80
POSTGRES_DB=fsqr
POSTGRES_USER=fsqr
FSQR_EMBEDDINGS_API_KEY=tei-local
TEI_IMAGE=ghcr.io/huggingface/text-embeddings-inference:cpu-1.9
TEI_PLATFORM=linux/amd64
TEI_MODEL_ID=intfloat/multilingual-e5-small
TEI_MODEL_REVISION=fd1525a9fd15316a2d503bf26ab031a61d056e98
TEI_SERVED_MODEL_NAME=intfloat/multilingual-e5-small
HF_TOKEN=
WATCHTOWER_INTERVAL=300
GHCR_USERNAME=
GHCR_TOKEN=
```

From repository root:

```sh
set -a; source .env; set +a
export HCLOUD_TOKEN="$HCLOUD_API_KEY"
export TF_VAR_hcloud_token="$HCLOUD_API_KEY"
```

Then:

```sh
cd deployment
tofu init
tofu fmt -recursive
tofu validate
tofu plan
```

Create resources only after reviewing the plan:

```sh
tofu apply
```

After apply, use the output SSH command. Domain DNS will be configured manually to the `ipv4_address` output.

## App Bootstrap

From repository root:

```sh
just bootstrap
```

The bootstrap command:

- Resolves the server IPv4 from `tofu output -raw ipv4_address`, unless `FSQR_DEPLOY_HOST` is set.
- Reads root `.env` as the only local secret source.
- Generates a minimal `/opt/fsqr/.env` for Docker Compose secrets/interpolation.
- Generates `/opt/fsqr/config.yaml` for application settings.
- Copies `build/docker-compose.prod.yml` as `/opt/fsqr/compose.yml`, plus `Caddyfile.prod` and SQL migrations.
- Optionally logs in to GHCR when `GHCR_USERNAME` and `GHCR_TOKEN` are present in root `.env`.
- Runs `docker compose pull` and starts the stack. It does not apply migrations.

The `fsqr` container receives application settings only through
`FSQR_CONFIG_FILE=/app/config/config.yaml`; production database and embedding
settings are not passed to the application as individual env vars.

Run migrations independently after bootstrap:

```sh
just migrate
```

The migrate command copies local `migrations/*.sql` to the server and runs the
production Compose `migrate` service. Use it before relying on Watchtower for
schema-changing releases.

Useful overrides:

```sh
FSQR_DEPLOY_HOST=203.0.113.10 just bootstrap
FSQR_DEPLOY_USER=deploy just bootstrap
FSQR_DEPLOY_DIR=/opt/fsqr just bootstrap
FSQR_ROOT_ENV_FILE=.env just bootstrap
```

Set `FSQR_DOMAIN` in root `.env` after DNS points to the server. The default `:80` serves HTTP only; a real domain enables Caddy-managed HTTPS.

## SSH Access

The deploy key lives in `../.ssh/` and is intentionally ignored by Git and Docker.

Default SSH exposure is `0.0.0.0/0` because no stable operator IP is known yet. To restrict it, pass a CIDR:

```sh
tofu apply -var='ssh_source_ips=["203.0.113.10/32"]'
```
