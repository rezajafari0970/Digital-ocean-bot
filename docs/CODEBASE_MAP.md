# Codebase Map

## Entrypoints
- `cmd/api`: production HTTP/Admin API composition.
- `cmd/worker`: production asynchronous worker.
- `cmd/admin-bootstrap`: admin bootstrap utility.
- `cmd/lifecycle-once`, `cmd/resume-one`, `cmd/ready-panel-worker`: lifecycle/repair utilities.
- `cmd/vultr-browser-session`, `cmd/vultr-capacity-probe`: Vultr interactive/capacity tooling.

## Core packages
- `internal/app`: dependency composition, lifecycle, deploy, recovery, scheduler, provider refresh, account state, proxy pool/sticky proxy.
- `internal/providers`: provider interfaces/registry/common errors and metadata.
- `internal/providers/digitalocean`: DO client, account/catalog/discovery/resources/mutations/pagination/DriverV2.
- `internal/providers/vultr`: Vultr API client plus read/mutation drivers.
- `internal/providers/vultrconsole`: browser session and reverse-proxy bridge for interactive console flows.
- `internal/droplets`: compute executor, lifecycle engine, reconcile and models.
- `internal/accounts`: account context, isolation cell and identity.
- `internal/network`: proxy gateway, isolation, egress guard, health, privacy, metadata and scheduler.
- `internal/adminapi`: Admin HTTP handlers for accounts, proxies, providers, resources, configs, Output, capacity, installers, residential and runtime/status.
- `internal/panels`: panel driver/store plus modular Sanaei/x-ui lifecycle.
- `internal/auth`, `internal/secrets`, `internal/audit`, `internal/observability`: cross-cutting security/operations.

## Panel subdomains
- `panels/sanaei`: API/auth/session/runtime/inbound/client/mutation/inventory/installer and resilient access.
- `panels/realityconfig`, `realitycontract`, `globalreality`: Reality generation/contracts/global behavior.
- `panels/policy`: inbound policy normalize/validate/allocate/expand/store.
- `panels/inventory`, `listeners`: discovered inbound state and parsing.
- `panels/create`, `update`, `desired`, `readyworker`: lifecycle orchestration.
- `panels/healthverify`: Xray health verification.
- `panels/usercapacity`: user slot/capacity filling.
- `panels/residentialsync`: residential routing synchronization.

## UI and deployment
- `web/static/index.html`: panel shell/markup.
- `web/static/app.js`: client behavior and API integration.
- `web/static/app.css`: visual system.
- `deploy/*.service`: systemd API/worker units.
- `deploy/install.sh`, `upgrade.sh`, `bootstrap.sh`, `healthcheck.sh`: deployment operations.

## Database
Migrations are append-only history under `migrations/`. Recent schema areas include Output snapshots/visibility, Ubuntu image restriction, account soft delete/archive, provider snapshots, proxy pool, provider capacity observations, and Vultr console capacity credentials. Always inspect the live migration level before assuming a migration is deployed.

## Reading strategy for a new session
For a feature, start at its Admin handler/UI call, trace into `internal/app` orchestration, then provider/panel/network implementation, then store/migration. For a runtime bug, inspect service/logs plus the exact deployed binary/source relationship. For provider work, never copy DigitalOcean semantics blindly into Vultr: use the common interface but preserve provider-specific discovery/capacity/error behavior.
