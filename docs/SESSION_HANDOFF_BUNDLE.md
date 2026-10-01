# SESSION HANDOFF BUNDLE

> Generated continuity entrypoint. Read this first; source/runtime remain authoritative.

Generated UTC: 2026-10-01T04:15:52.274018+00:00
Branch: `checkpoint/final-e2e-20260929`
HEAD at bundle generation: `03f810813368f1902cb85f42d8de0d9d8d48c198`

## Mandatory startup procedure
1. Read this bundle completely.
2. Run `tools/check-project-continuity.sh`.
3. Read `HANDOFF.md` and inspect the exact source files/commits relevant to Current Work.
4. Verify production/runtime and live migration state before mutation.
5. Continue from the stated boundary; do not redesign from zero.

## Current State
# Project State — Digital-ocean-bot
Updated: 2026-10-01
Repository: /root/projects/Digital-ocean-bot-canonical-e2e
Remote: git@github.com:rezajafari0970/Digital-ocean-bot.git
Branch: checkpoint/final-e2e-20260929
HEAD: 6834d813fc16a6b4aefd05f2cb8696c18b0c0fec

## Runtime
- digital-ocean-bot-api: active
- digital-ocean-bot-worker: active
- Production runtime: /opt/digital-ocean-bot

## Current Vultr state
- Interactive Vultr console / reverse-proxy / noVNC work is deployed.
- Short-lived authenticated HttpOnly browser session exists for console tabs.
- noVNC websocket proxy path was corrected.
- Root noVNC websocket alias is accepted by commit 6834d81.
- Recent chain: 7b79dad -> 7c9f335 -> af598d7 -> 7126567 -> 6834d81.

## Current verification target
Refresh Admin, open Vultr Console, and verify the fresh ticket/session reaches interactive noVNC. Cloudflare/CAPTCHA/2FA remains a manual browser step when presented.

## Exact Handoff
# Handoff
Updated: 2026-10-01

## Resume here
Current workstream: Vultr interactive console access through reverse proxy/noVNC.
Latest: 6834d81 — accept root noVNC websocket alias.
Previous: 7126567 websocket path; af598d7 short session cookie; 7c9f335 interactive challenge session; 7b79dad console capacity browser probe.

## Exact next action
1. Refresh/re-login Admin so obsolete pre-restart session is not reused.
2. Click Open Vultr Console for a fresh ticket/session.
3. Verify noVNC leaves connecting state and WebSocket upgrade succeeds.
4. Solve Cloudflare/CAPTCHA/2FA manually if presented.
5. Verify the authorized Vultr session can be reused by capacity/account workflow.

## Working tree warning
Do not delete/reset .audit blindly. At checkpoint creation source matched origin at 6834d81; .audit had modified/untracked analysis artifacts. Treat them as audit evidence, not source changes.

## Master Context
# ChatGPT Master Context

## Mission
Build and operate one modular automation platform for cloud accounts, compute lifecycle, proxy/network identity, Sanaei/x-ui deployment, Reality configuration, live config Output, and provider-specific capacity discovery. DigitalOcean is the mature baseline; Vultr is being integrated without weakening provider isolation.

## Source of truth
Canonical source: `/root/projects/Digital-ocean-bot-canonical-e2e` on `serverprojects.ptr.network`. Never assume GitHub main is production truth. On every new session read this file plus `PROJECT_CONTEXT.md`, `PROJECT_STATE.md`, `HANDOFF.md`, then verify HEAD/status/services before edits.

## Current scale
The Go codebase currently contains about 408 Go files, 121 test files and 102 up migrations. Web UI is `web/static/{index.html,app.js,app.css}`. Runtime entrypoints live under `cmd/`; API and worker are the primary services.

## Architectural spine
`cmd/api` wires the Admin/API surface. `cmd/worker` runs asynchronous lifecycle work. `internal/app` is orchestration/composition. `internal/providers` is the cloud-provider abstraction and registry. Provider implementations are isolated under `digitalocean`, `vultr`, and browser-console support under `vultrconsole`. `internal/droplets` owns compute lifecycle/reconciliation. `internal/network` owns proxy/egress isolation and health. `internal/accounts` owns account context/cells/identity. `internal/panels` owns panel lifecycle and Sanaei integration. `internal/adminapi` exposes management APIs consumed by the web UI. PostgreSQL schema evolution is in `migrations/`.

## Provider model
DigitalOcean implementation includes account discovery, catalog, pagination, resources, mutations, error classification and DriverV2. Vultr has separate client/read/mutation drivers and types. Vultr console/browser automation is deliberately separate from API provider logic and exists to support interactive browser-only flows and capacity discovery when authoritative API information is insufficient.

## Panel/Sanaei model
Panel logic is modular: create/update orchestration, desired state, inventory, listeners, policy allocation, Reality contract/scan/config builder, health verification, user capacity, residential sync and Sanaei driver/runtime/session/API modules. Live Output should be derived from Sanaei/API state, not SSH scraping. SSH remains an installation/session capability where required, not the Output data plane.

## Product behavior
Accounts and proxies are independent modules. Account/provider state must be explicit and observable. Proxy assignment supports account isolation/sticky behavior and health handling. Server creation is scheduler/lifecycle driven, capacity-aware, and reconciled rather than treated as a one-shot script. Panel installation/configuration follows compute readiness. Reality/VLESS is the primary current config path. Output is intended to update rapidly and suppress expired/soon-invalid entries using persisted visibility/expiry semantics.

## Safety of project state
Never reset or delete uncommitted/audit material without inspection. Never put tokens, passwords, cookies, API keys, private keys or session secrets into continuity docs or Git. Preserve migrations and deployed-state compatibility. Before deploy: tests/build/migration compatibility. After deploy: service health plus functional smoke test. Every completed logical stage gets commit + push + continuity-doc update.

## Current Vultr workstream
Vultr capacity must be determined accurately, analogous in purpose to DigitalOcean capacity but using Vultr-specific authoritative behavior. Interactive Vultr console access was added through a reverse proxy/noVNC flow so a human can solve Cloudflare/CAPTCHA/2FA when required. Recent commits added capacity browser probe, challenge session, short browser session cookie, corrected noVNC websocket routing, and root websocket alias support. Current verification is a fresh Admin login/session -> Open Vultr Console -> fresh ticket -> successful WebSocket/noVNC connection -> manual challenge if shown -> reuse authorized state for capacity/account workflow.

## How the next ChatGPT session should behave
Do not restart design from zero. First establish current truth from Git and production. Read relevant code before proposing edits. Preserve existing modular architecture and naming. When the user says “next logical step,” continue from HANDOFF, test the current boundary, then implement the smallest complete production-grade step including tests, deploy/checkpoint where appropriate. When debugging, inspect actual logs/code/runtime rather than guessing. Keep the user informed of concrete results, not hypothetical progress.

## Architecture / Codebase Map
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

## Feature Flow Index
# Feature Flow Index

This is the navigation index for a new ChatGPT session. It maps user-visible features to API, implementation domains, persistence and runtime boundaries. Exact behavior remains authoritative in source.

## Accounts
UI: `web/static/app.js` Accounts page/forms/dashboard. API registration: `internal/adminapi/server.go`; handlers split across `accounts_manage.go`, `accounts_write.go`, `account_preview.go`, `account_validation.go`, `account_options.go`, `account_identity.go`, `account_preflight.go`, `account_automation.go`, `discovery.go`, `dashboard.go`. Orchestration/state: `internal/app/account_state.go`, `provider_refresh.go`, `network_identity.go`, `sticky_proxy.go`, `proxy_pool.go`; isolation primitives: `internal/accounts/*`. Provider execution enters `internal/providers/*`. Account schema history is in migrations; inspect live version before assumptions.

## Provider abstraction / DigitalOcean / Vultr
Common contracts and registry: `internal/providers/provider.go`, `types.go`, `registry.go`, `errors.go`, `metadata.go`. DigitalOcean: `internal/providers/digitalocean/*` with account/catalog/discovery/resources/mutations/pagination/DriverV2. Vultr API: `internal/providers/vultr/*` with client/read/mutation drivers. Provider metadata endpoint: `GET /api/v1/providers` -> `internal/adminapi/providers.go`. Account discovery/refresh endpoints flow through admin handlers into provider-specific drivers.

## Vultr interactive console and capacity
UI action `data-action=vultr-console` in `web/static/app.js` calls account console-session flow. Route `POST /api/v1/accounts/{id}/console-session` -> `createVultrBrowserTicket` in `internal/adminapi/server.go`. Browser traffic routes through `GET /vultr-browser/{path...}` and root `GET /websockify` -> `vultrBrowserProxy`. Browser/noVNC implementation: `internal/providers/vultrconsole/browser.go` and `proxy_bridge.go`. Utilities: `cmd/vultr-browser-session`, `cmd/vultr-capacity-probe`. Persistence for console capacity credentials/observations: recent migrations `000100_provider_capacity_observations` and `000101_vultr_console_capacity_credentials`. Current boundary is functional WebSocket/noVNC verification with a fresh session.

## Proxies and account network assignment
UI Proxies page -> `/api/v1/proxies*`; handlers: `proxies_manage.go`, `proxies_write.go`, `proxy_details.go`, `proxy_detect.go`, `proxy_health.go`, `proxy_observe.go`, `proxy_pool.go`. Account assignment/test: `PUT /api/v1/accounts/{id}/network`, `POST .../proxy-test`. Runtime/network enforcement: `internal/network/*`; orchestration: `internal/app/proxy_provider.go`, `proxy_pool.go`, `sticky_proxy.go`, `network_identity.go`. Keep account isolation and health/failure behavior intact when changing this path.

## Compute build lifecycle
Core lifecycle: `internal/droplets/executor.go`, `lifecycle.go`, `lifecycle_engine.go`, `reconcile.go`, `factory.go`. App orchestration: `internal/app/lifecycle.go`, `scheduler.go`, `deploy.go`, `recovery.go`, `deployment_ready.go`, `orphan_reconcile.go`. Worker entry: `cmd/worker`. Admin observability: `/api/v1/build-activity*`, `/api/v1/provision-errors` -> `build_activity.go`, `provision_activity.go`, `provision_errors.go`. UI has a live Build Activity renderer in `web/static/app.js`.

## Sanaei/x-ui installation and runtime
Admin installer endpoints in `server.go` -> `sanaei_installer.go`, `installers.go`, `install_scripts.go`, `installer_selection.go`, `installer_rearm.go`. Panel orchestration: `internal/panels/create`, `update`, `desired`, `readyworker`, `panelbootstrap`. Sanaei implementation: `internal/panels/sanaei/*` including API/auth/session/runtime/manager/inbounds/clients/mutations/inventory/installer. SSH is installation/session infrastructure where needed; do not redesign live Output to scrape through SSH.

## Reality Config policy and capacity
UI Configs page -> `GET/PUT /api/v1/configs` and `GET /api/v1/config-capacity`; handlers: `configs_global.go`, `configs.go`, `config_capacity.go`, `config_expr.go`. Panel policy: `internal/panels/policy/*`; Reality construction/contract: `realityconfig`, `realitycontract`, `globalreality`; scanning is in Sanaei Reality modules. User capacity: `internal/panels/usercapacity/*`. Destructive cleanup route starts `POST /api/v1/config-capacity/delete-all-clients` and polls cleanup job status.

## Output and share links
UI Output page uses `POST /api/v1/output/share`, share URLs, and direct `GET /api/v1/output`; supports all, near-expiry and recently-created filters. Handlers: `internal/adminapi/output.go`, `output_snapshot.go`; runtime manager: `internal/panels/sanaei/runtime_manager.go` and related Sanaei API/session code. `cmd/api/main.go` warms Output cache on startup and periodically. Persistence evolved through `000090_output_snapshots`, `000091_inbound_structural_snapshots`, `000092_output_visibility`. Visibility/expiry semantics must be preserved so invalid configs disappear on time.

## Residential routing
UI Residential -> `/api/v1/residential-proxies*`; handler `internal/adminapi/residential.go`; panel synchronization `internal/panels/residentialsync/service.go`; relevant Sanaei traffic modules include `traffic.go`/`traffic_adapter.go`. Migration `000086_residential_proxies` established persistence. Changes must preserve separation from ordinary account proxy assignment.

## Admin UI/auth/settings
Static UI: `web/static/index.html`, `app.js`, `app.css`. Login/logout routes in `server.go`; auth handler `internal/adminapi/auth.go`; auth core `internal/auth/*`. Panel settings: `panel_settings.go`. API security middleware: `internal/adminapi/security.go`. Remember-me/session behavior is implemented across UI token storage and backend auth; inspect both before changing.

## Deployment/runtime
API composition: `cmd/api/main.go`; worker: `cmd/worker/main.go`. systemd definitions and deployment scripts: `deploy/`. Canonical source and production deployment can differ temporarily, so always verify deployed state before claiming a code change is live. Migrations are in `migrations/` and must be checked against live DB.

## Historical Decisions
# Historical Decision Ledger

Purpose: preserve project intent and lessons that cannot be reconstructed reliably from source code alone. Source/runtime still wins for exact implementation state.

## Architecture decisions
- Cloud providers are plugins/drivers behind a provider-neutral Registry/Factory and canonical models. Scheduler/Lifecycle/Core must not accumulate DigitalOcean/Vultr branches.
- DigitalOcean is the mature reference implementation, but Vultr is not a clone: provider-specific catalog, credential, pagination, capacity and error semantics stay inside its driver.
- Canonical lifecycle is `Account -> Proxy -> Provider -> Server -> Deployment -> Provisioning -> Sanaei -> Panel`.
- Proxy is an independent module. Account egress is isolated and fail-closed; shared proxy/network identity between accounts is a regression.
- Scheduler/UI enqueue work; execution must avoid competing deploy executors. Historical duplicate-installer failures showed that in-process locks are insufficient across processes. Long-running install work belongs to controlled worker execution with per-resource coordination and visible/streamed diagnostics.

## DigitalOcean baseline decisions
- DO must remain fully functional while provider-neutralization proceeds; new provider work must not regress it.
- Provider observations are intentionally focused on account/catalog/servers rather than accumulating unrelated browser state.
- Provider errors are classified rather than treated as generic failures; retired/unavailable images must be recognized as image availability failures rather than retried blindly.
- Desired defaults established during provider cleanup: 5 region choices, 3 plan choices, Ubuntu-only image choices (historically 22.04/24.04/26.04 where actually provider-available), and random server lifetime around 90–120 minutes. Availability from the provider remains authoritative.
- Optional account email/password browser credentials must not replace API-token behavior. When absent, token-only behavior remains unchanged.

## Vultr integration decisions
- Vultr must use the same provider-neutral core but its own authoritative read/mutation/capacity behavior.
- Known integration traps that must not return: DO-only image validation in generic paths; unknown capacity blocking StartDeployment; UI hiding Vultr as development-only; reconciliation hard-coding DigitalOcean; artificial Vultr pagination ceilings; assuming Vultr credential lifetime/shape equals DO.
- Vultr reached an E2E server lifecycle through READY/PANEL_COMPLETE during integration; provisioning used public SSH-key/cloud-init readiness and Sanaei API for panel operations.
- Accurate Vultr account capacity is a first-class requirement. Where normal API data is insufficient, browser-console observation is isolated in `vultrconsole` rather than leaking browser logic into the provider-neutral core.
- Cloudflare/CAPTCHA/2FA is human-completed in an interactive console. The system may establish/reuse the resulting authorized session; it should not turn challenge handling into hidden core/provider logic.

## Proxy / identity decisions
- Each account must have an independently managed network identity; accidental shared DataImpulse/proxy state across accounts was identified as an isolation violation.
- Proxy type detection, health, sticky assignment and failure behavior belong to proxy/network modules, not cloud-provider drivers.
- A failed required proxy must fail closed rather than silently leaking direct provider traffic.
- Provider/account UI should expose actionable state classes rather than a generic red/green result where possible.

## Compute / provisioning decisions
- Server lifecycle is reconciled state, not a one-shot shell script. Creation, readiness, installation and panel completion have observable stages and retry/error classification.
- Historical architecture with multiple schedulers/workers/manual threads caused duplicate installs and stale-snapshot races. Do not reintroduce parallel owners of the same deployment state machine.
- Installation diagnostics must remain visible enough to identify SSH/script phase, retries and recurring sanitized error fingerprints.
- Provider resource creation and panel installation are separate boundaries: provider success does not imply panel readiness.

## Sanaei / Reality decisions
- Sanaei/x-ui is managed as a modular panel driver/runtime. Live panel/config reads should use Sanaei API where implemented; SSH is not the live Output data plane.
- Reality/VLESS is the current primary generated config path. Multiple inbounds/ports and SNI target selection are policy-driven rather than hard-coded into provider lifecycle.
- SNI/Reality target selection must use the dedicated target-finding/scan logic rather than arbitrary fixed targets when automatic selection is requested.
- User quota, lifetime, device limit and target users per inbound are policy controls and must remain independently configurable.

## Output decisions
- Output must feel live and fast. It must not become a slow SSH aggregation path.
- Snapshot persistence exists for performance/continuity, but stale visibility is not acceptable: expired or scheduled-invalid configs must disappear according to persisted visibility/expiry semantics.
- Earlier requirement established hiding configs shortly before scheduled deletion/expiry rather than waiting for a coarse estimated TTL. Do not regress to an approximate cache-only lifetime.
- Share links and direct Output views must apply the same visibility rules. Filters such as near-expiry/recently-created are views over the canonical valid set, not alternate stale stores.

## Residential routing decisions
- Residential egress is a separate panel-routing concern from account/provider proxy assignment.
- Residential routing was validated as working and is synchronized into Sanaei routing; keep country/egress behavior isolated from ordinary cloud account control-plane traffic.

## UI / operational decisions
- The Admin panel is mobile-friendly and operational: Accounts, Proxies, Residential, Configs, Output and settings are separate user-facing modules.
- Account edit/save/details and Output operations are expected to be fast and not jump/reset the user's working context unnecessarily.
- Remember-me behavior was intended to persist login for an extended period; frontend token storage and backend auth/session behavior must be considered together.
- Human challenge pages need an interactive browser path, not a fake success status or a background wait with no feedback.

## Development/process decisions
- Read the real source before making architectural changes. Audit work should be staged and evidence-preserving; do not mutate production during an audit-only phase.
- Canonical server repository/branch, GitHub checkpoint and production runtime must be distinguished explicitly.
- Never reset `.audit` material blindly. Never put credentials/session secrets in Git or continuity docs.
- Every completed logical stage should have focused tests, appropriate broader verification, commit/push, deploy verification when deployed, and an updated HANDOFF/PROJECT_STATE.

## Rejected/regressive patterns
- Provider conditionals scattered through Scheduler/Lifecycle/Core.
- Treating Vultr as a renamed DigitalOcean implementation.
- Shared account egress identity where isolation is required.
- Silent direct-network fallback after required proxy failure.
- Multiple independent executors owning the same install/deployment lifecycle.
- SSH scraping as the live Output source.
- Approximate Output TTL when exact client expiry/deletion timing is available.
- Hard-coded provider image assumptions in generic validation.
- Declaring a feature complete from source changes alone without production/runtime verification.

## Requirements / Invariants
# Requirements Matrix

| Area | Invariant / intended behavior | Primary code ownership |
|---|---|---|
| Provider core | Provider-neutral Registry/Factory; no DO/Vultr branches in generic lifecycle | `internal/providers`, `internal/app` |
| DigitalOcean | Mature baseline stays working; provider-specific catalog/errors/pagination inside driver | `internal/providers/digitalocean` |
| Vultr | Independent read/mutation/capacity semantics; no DO clone | `internal/providers/vultr` |
| Vultr console | Human-interactive challenge/noVNC path isolated from generic provider core | `internal/providers/vultrconsole`, Admin API |
| Account network | Per-account isolated proxy identity; required proxy failure is fail-closed | `internal/accounts`, `internal/network`, `internal/app` |
| Lifecycle | One coherent state-machine owner; reconciled/observable stages | `internal/droplets`, worker/app lifecycle |
| Sanaei | API-driven live panel operations where implemented; SSH not Output read plane | `internal/panels/sanaei` |
| Reality | Policy-driven VLESS/Reality ports/SNI/users/quota/lifetime/device limits | `internal/panels/policy`, Reality modules |
| Output | Fast/live canonical set; exact visibility/expiry; share/direct rules match | `internal/adminapi/output*`, Sanaei runtime |
| Residential | Separate panel data-plane routing from account control-plane proxy | `residentialsync`, `adminapi/residential.go` |
| UI | Fast operational module pages; preserve user context; interactive challenge feedback | `web/static/*`, `internal/adminapi` |
| Continuity | Git/source/runtime truth + docs; test/deploy/checkpoint each completed stage | repo docs/deploy workflow |

## Change Protocol
# Change Protocol

## Before touching a feature
1. Locate it in `FEATURE_FLOW_INDEX.md`.
2. Read the UI caller, registered API route and handler.
3. Trace orchestration into provider/panel/network/store code.
4. Read related tests and migrations.
5. Verify current production state/logs if the issue is runtime-visible.

## During implementation
Keep provider-specific behavior behind provider boundaries. Do not bypass account/network isolation. Do not replace Sanaei API live reads with SSH. Do not introduce stale Output behavior. Do not store secrets in Git/docs/logs. Prefer a complete vertical slice over unrelated refactors.

## Verification
Run focused package tests first, then broader tests/build appropriate to the change. For schema changes, verify migration ordering and live migration level. For web/API changes, exercise the real endpoint/UI flow. For deploys, verify API and worker service health plus the feature smoke test.

## Commit/checkpoint
A logical stage is not complete until code/tests are committed, pushed to the canonical checkpoint branch, and `PROJECT_STATE.md` + `HANDOFF.md` reflect the exact deployed/tested boundary. Audit artifacts are preserved unless deliberately reviewed and archived.

## Machine Snapshot
```json
{
  "branch": "checkpoint/final-e2e-20260929",
  "head": "03f810813368f1902cb85f42d8de0d9d8d48c198",
  "services": {
    "digital-ocean-bot-api": "active",
    "digital-ocean-bot-worker": "active"
  },
  "inventory": {
    "go_files": 408,
    "tests": 121,
    "up_migrations": 102
  },
  "route_count": 61,
  "recent_migrations": [
    "000090_output_snapshots.up.sql",
    "000091_inbound_structural_snapshots.up.sql",
    "000092_output_visibility.up.sql",
    "000093_ubuntu_images_only.up.sql",
    "000094_account_soft_delete.up.sql",
    "000095_finalize_pending_account_archives.up.sql",
    "000096_remove_account_login_browser_legacy.up.sql",
    "000097_provider_snapshot_canonical.up.sql",
    "000098_remove_unused_trafficguard.up.sql",
    "000099_account_proxy_pool.up.sql",
    "000100_provider_capacity_observations.up.sql",
    "000101_vultr_console_capacity_credentials.up.sql"
  ],
  "current_work": {
    "area": "Vultr interactive console/capacity",
    "verification_boundary": "Fresh Admin session -> Open Vultr Console -> fresh ticket -> successful noVNC WebSocket -> manual challenge if presented -> reuse authorized state for capacity workflow",
    "handoff": "HANDOFF.md"
  },
  "recent_commits": [
    "e986b92 (HEAD -> checkpoint/final-e2e-20260929, origin/checkpoint/final-e2e-20260929) tools: automate project continuity snapshots",
    "add585c docs: add machine-readable project continuity manifest",
    "d6d4caa docs: preserve historical decisions and subsystem invariants",
    "7b49e77 docs: index feature flows and change protocol",
    "d44242e docs: add master architecture and session bootstrap context",
    "80d29ac docs: add durable project continuity checkpoint",
    "6834d81 fix(vultr): accept root noVNC websocket alias",
    "7126567 fix(vultr): correct noVNC websocket path behind proxy"
  ]
}
```

## Source-of-truth rule
This bundle is an accelerator, not a substitute for code. If prose conflicts with Git source, live DB migration state, service logs or production runtime, investigate the drift and update continuity records after resolving it.
