# Deployment / Runtime Map

Canonical source: `/root/projects/Digital-ocean-bot-canonical-e2e` on branch `checkpoint/final-e2e-20260929` at `fa815e643a71d3623d2abce17058d2f725a00f4e`.

## Production layout
- App: `/opt/digital-ocean-bot`
- Binaries: `/opt/digital-ocean-bot/bin`
- Environment file: `/etc/digital-ocean-bot/env` (never copy secrets into docs)
- Mutable data: `/var/lib/digital-ocean-bot`
- Static UI: `/opt/digital-ocean-bot/web/static`
- Migrations copy: `/opt/digital-ocean-bot/migrations`

## Services
### `digital-ocean-bot-api`
- State: `active/running`; PID `2532688`
- Unit: `/etc/systemd/system/digital-ocean-bot-api.service`
- Working directory: `/opt/digital-ocean-bot`
- Executable: `/opt/digital-ocean-bot/bin/digital-ocean-bot-api`
- Binary SHA256: `2f8eb59be1ce359a0fd9500ac741f3b2e717c0b7e1c08662b00e7f7ef9c78598`

### `digital-ocean-bot-worker`
- State: `active/running`; PID `2137080`
- Unit: `/etc/systemd/system/digital-ocean-bot-worker.service`
- Working directory: `/opt/digital-ocean-bot`
- Executable: `/opt/digital-ocean-bot/bin/digital-ocean-bot-worker`
- Binary SHA256: `7bd4042953d7f5d6f43a53311ef0b2d600b0e72e293ca21b18e7d0159cd56fcb`

## Build / deploy path
- `deploy/install.sh` builds API/worker directly from source into `/opt/digital-ocean-bot/bin`, copies migrations/static assets, installs systemd units and enables services.
- `deploy/upgrade.sh` stops services, copies migrations/static assets, builds `.new` binaries, atomically replaces production binaries, then starts services.
- `deploy/healthcheck.sh` checks `/healthz`, `/readyz`, API active and worker active.

## Revision identity caveat
The existing build scripts do **not** stamp the Git commit into the binary. Therefore “service active” does not prove production is running the current repository HEAD. Before claiming a deploy is live, use the actual deployment procedure plus health/feature smoke verification. A future hardening step can add an artifact build manifest or version endpoint mapping binary SHA/commit/deploy time.

Full machine-readable runtime snapshot: `docs/RUNTIME_MAP.json`.
