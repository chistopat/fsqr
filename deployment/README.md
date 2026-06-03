# fsqr Hetzner Deployment

OpenTofu configuration for one Hetzner Cloud VM in Helsinki with Docker CE preinstalled.

## Resources

- Firewall: inbound TCP `22`, `80`, `443` only.
- Static Primary IPv4: `auto_delete = false` so DNS can keep using the same address after VM replacement.
- Server: `cpx42` in `hel1` Helsinki, image `docker-ce`, IPv4 enabled, IPv6 disabled.
- SSH key: uploaded from `../.ssh/fsqr_hcloud_ed25519.pub`.
- Cloud-init: creates non-root `deploy` user, disables password auth and root SSH, verifies Docker Compose.

## Commands

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
- Creates ignored `deployment/.env.deploy` on first run with a random Postgres password.
- Copies `compose.prod.yml`, `Caddyfile.prod`, `.env.deploy`, and SQL migrations to `/opt/fsqr`.
- Optionally logs in to GHCR when `GHCR_USERNAME` and `GHCR_TOKEN` are present in `.env.deploy`.
- Runs `docker compose pull`, applies migrations, and starts the stack.

Useful overrides:

```sh
FSQR_DEPLOY_HOST=203.0.113.10 just bootstrap
FSQR_DEPLOY_USER=deploy just bootstrap
FSQR_DEPLOY_DIR=/opt/fsqr just bootstrap
FSQR_DEPLOY_ENV_FILE=deployment/.env.deploy just bootstrap
```

Set `FSQR_DOMAIN` in `deployment/.env.deploy` after DNS points to the server. The default `:80` serves HTTP only; a real domain enables Caddy-managed HTTPS.

## SSH Access

The deploy key lives in `../.ssh/` and is intentionally ignored by Git and Docker.

Default SSH exposure is `0.0.0.0/0` because no stable operator IP is known yet. To restrict it, pass a CIDR:

```sh
tofu apply -var='ssh_source_ips=["203.0.113.10/32"]'
```
