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
