# Deployment / Runtime Map

Canonical source: `/root/projects/Digital-ocean-bot-canonical-e2e` on branch `checkpoint/final-e2e-20260929` at `03f810813368f1902cb85f42d8de0d9d8d48c198`.

## Production layout
- App: `/opt/digital-ocean-bot`
- Binaries: `/opt/digital-ocean-bot/bin`
- Environment file: `/etc/digital-ocean-bot/env` (never copy secrets into docs)
- Mutable data: `/var/lib/digital-ocean-bot`
- Static UI: `/opt/digital-ocean-bot/web/static`
- Migrations copy: `/opt/digital-ocean-bot/migrations`

## Services
### `digital-ocean-bot-api`
- State: `active/running`; PID `2542899`
- Unit: `/etc/systemd/system/digital-ocean-bot-api.service`
- Working directory: `/opt/digital-ocean-bot`
- Executable: `/opt/digital-ocean-bot/bin/digital-ocean-bot-api`
- Binary SHA256: `c1966b097fb039ff9547e55adbc79bc7e27740885652226274fb88d205487450`

### `digital-ocean-bot-worker`
- State: `active/running`; PID `2542900`
- Unit: `/etc/systemd/system/digital-ocean-bot-worker.service`
- Working directory: `/opt/digital-ocean-bot`
- Executable: `/opt/digital-ocean-bot/bin/digital-ocean-bot-worker`
- Binary SHA256: `6ff944cf74339d0c65dd7a1fb03ac3e45d3f5601dec153adf65f174db72d3840`

## Build / deploy path
- `deploy/install.sh` builds API/worker directly from source into `/opt/digital-ocean-bot/bin`, copies migrations/static assets, installs systemd units and enables services.
- `deploy/upgrade.sh` stops services, copies migrations/static assets, builds `.new` binaries, atomically replaces production binaries, then starts services.
- `deploy/healthcheck.sh` checks `/healthz`, `/readyz`, API active and worker active.

## Revision identity caveat
The existing build scripts do **not** stamp the Git commit into the binary. Therefore “service active” does not prove production is running the current repository HEAD. Before claiming a deploy is live, use the actual deployment procedure plus health/feature smoke verification. A future hardening step can add an artifact build manifest or version endpoint mapping binary SHA/commit/deploy time.

Full machine-readable runtime snapshot: `docs/RUNTIME_MAP.json`.
