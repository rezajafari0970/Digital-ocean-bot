# Requirement Traceability Matrix

Requirement → Code → Symbol → DB → Migration → Test → Runtime

| ID | Area | Files | Symbols | DB | Migrations | Tests | Runtime |
|---|---|---:|---:|---:|---:|---:|---:|
| `ARCH-001` | architecture | 78 | 364 | 27 | 51 | 20 | 2 |
| `ACC-001` | accounts | 8 | 21 | 10 | 31 | 4 | 0 |
| `NET-001` | proxy_network | 36 | 102 | 6 | 25 | 17 | 0 |
| `LIFE-001` | lifecycle | 13 | 42 | 11 | 31 | 5 | 24 |
| `DO-001` | digitalocean | 16 | 97 | 0 | 0 | 6 | 0 |
| `VULTR-001` | vultr | 11 | 70 | 0 | 0 | 4 | 0 |
| `VULTR-002` | vultr_capacity | 5 | 19 | 5 | 24 | 2 | 0 |
| `VULTR-003` | vultr_console | 4 | 17 | 1 | 19 | 2 | 0 |
| `PANEL-001` | sanaei | 51 | 200 | 7 | 9 | 17 | 0 |
| `OUT-001` | output | 3 | 29 | 11 | 33 | 1 | 0 |
| `OUT-002` | output_expiry | 2 | 18 | 11 | 33 | 0 | 0 |
| `REAL-001` | reality | 10 | 15 | 7 | 13 | 3 | 0 |
| `RES-001` | residential | 2 | 11 | 5 | 13 | 0 | 0 |
| `UI-001` | frontend | 0 | 0 | 0 | 0 | 0 | 0 |
| `OPS-001` | production | 1 | 2 | 0 | 0 | 0 | 0 |
| `KNOW-001` | continuity | 0 | 0 | 0 | 0 | 0 | 0 |

## Coverage summary

- **requirements**: 16
- **with_code**: 14
- **with_symbols**: 14
- **with_database**: 11
- **with_migrations**: 11
- **with_tests**: 11
- **with_runtime**: 2

## Requirement details

### ARCH-001 — architecture

Keep cloud-provider core provider-neutral; provider-specific behavior stays in drivers.

**Why:** DigitalOcean remains the mature baseline while Vultr and future providers integrate without branching generic lifecycle code.

**Code:**
- `internal/app/account_state.go`
- `internal/app/account_state_test.go`
- `internal/app/bootstrap.go`
- `internal/app/capacity_guard.go`
- `internal/app/capacity_guard_test.go`
- `internal/app/catalog_sync.go`
- `internal/app/config.go`
- `internal/app/consistency.go`
- `internal/app/container.go`
- `internal/app/deploy.go`
- `internal/app/deployment_ready.go`
- `internal/app/installer_activation.go`
- `internal/app/lifecycle.go`
- `internal/app/network_identity.go`
- `internal/app/orphan_reconcile.go`
- `internal/app/panel.go`
- `internal/app/postinstall.go`
- `internal/app/postinstall_capabilities.go`
- `internal/app/postinstall_rearm.go`
- `internal/app/postinstall_reconcile.go`
- `internal/app/profile.go`
- `internal/app/provider_refresh.go`
- `internal/app/proxy_pool.go`
- `internal/app/proxy_provider.go`
- `internal/app/proxy_provider_test.go`
- `internal/app/recovery.go`
- `internal/app/repository.go`
- `internal/app/scheduler.go`
- `internal/app/ssh_identity.go`
- `internal/app/sticky_proxy.go`
- `internal/app/workflow.go`
- `internal/droplets/compute_stub_test.go`
- `internal/droplets/executor.go`
- `internal/droplets/executor_test.go`
- `internal/droplets/factory.go`
- `internal/droplets/lifecycle.go`
- `internal/droplets/lifecycle_engine.go`
- `internal/droplets/model.go`
- `internal/droplets/postgres_e2e_test.go`
- `internal/droplets/reconcile.go`
- `internal/droplets/reconcile_test.go`
- `internal/droplets/safety_characterization_test.go`
- `internal/providers/digitalocean/account.go`
- `internal/providers/digitalocean/catalog.go`
- `internal/providers/digitalocean/characterization_test.go`
- `internal/providers/digitalocean/client.go`
- `internal/providers/digitalocean/client_test.go`
- `internal/providers/digitalocean/discovery.go`
- `internal/providers/digitalocean/driver_v2.go`
- `internal/providers/digitalocean/driver_v2_test.go`
- `internal/providers/digitalocean/errors.go`
- `internal/providers/digitalocean/errors_parse_test.go`
- `internal/providers/digitalocean/errors_test.go`
- `internal/providers/digitalocean/mutations.go`
- `internal/providers/digitalocean/pagination.go`
- `internal/providers/digitalocean/pagination_regression_test.go`
- `internal/providers/digitalocean/resources.go`
- `internal/providers/digitalocean/types.go`
- `internal/providers/errors.go`
- `internal/providers/metadata.go`
- `internal/providers/provider.go`
- `internal/providers/provider_v2_test.go`
- `internal/providers/registry.go`
- `internal/providers/types.go`
- `internal/providers/vultr/client.go`
- `internal/providers/vultr/client_test.go`
- `internal/providers/vultr/driver.go`
- `internal/providers/vultr/driver_test.go`
- `internal/providers/vultr/mutation.go`
- `internal/providers/vultr/mutation_driver.go`
- `internal/providers/vultr/mutation_driver_test.go`
- `internal/providers/vultr/read.go`
- `internal/providers/vultr/read_driver.go`
- `internal/providers/vultr/read_driver_test.go`
- `internal/providers/vultr/types.go`
- `internal/providers/vultrconsole/browser.go`
- `internal/providers/vultrconsole/browser_test.go`
- `internal/providers/vultrconsole/proxy_bridge.go`

**Database:**
- `SET`
- `account_capacity_events`
- `account_network_identities`
- `account_proxy_pool`
- `accounts`
- `deployment_events`
- `deployment_installer_selections`
- `deployment_profiles`
- `deployments`
- `droplets`
- `installer_runs`
- `inventory`
- `lifecycle_events`
- `network_profiles`
- `operations`
- `panel_settings`
- `provider_capacity_observations`
- `provider_catalog_cache`
- `provider_snapshots`
- `provision_runs`
- `provision_step_attempts`
- `proxies`
- `resources`
- `schedules`
- `xui_database_deployments`
- `xui_database_templates`
- `xui_panel_deployments`

**Migrations:**
- `000001_core.up.sql`
- `000002_operations.up.sql`
- `000004_proxy_health.up.sql`
- `000006_provider_registry.up.sql`
- `000007_droplet_lifecycle.up.sql`
- `000009_provisioning.up.sql`
- `000010_xui_templates.up.sql`
- `000013_deployments.up.sql`
- `000014_deployment_profiles.up.sql`
- `000015_lifecycle_expiration.up.sql`
- `000019_schedules.up.sql`
- `000020_concurrency_hardening.up.sql`
- `000022_panel_settings.up.sql`
- `000023_account_automation.up.sql`
- `000024_account_server_choices.up.sql`
- `000026_proxy_health_successes.up.sql`
- `000027_provider_catalog_cache.up.sql`
- `000029_account_lifecycle_policy.up.sql`
- `000030_replacement_lifecycle.up.sql`
- `000031_account_runtime_status.up.sql`
- `000034_account_desired_server_count.up.sql`
- `000036_account_any_region_fallback.up.sql`
- `000038_capacity_history.up.sql`
- `000041_account_network_identity.up.sql`
- `000042_randomized_deployment_choices.up.sql`
- `000043_build_spacing.up.sql`
- `000044_build_spacing_range.up.sql`
- `000046_digitalocean_login_credentials.up.sql`
- `000047_build_spacing_max_default.up.sql`
- `000048_sticky_proxy_identity.up.sql`
- `000049_provision_retry_diagnostics.up.sql`
- `000051_account_browser_runtime.up.sql`
- `000054_browser_login_state.up.sql`
- `000055_login_challenge_state.up.sql`
- `000056_proxy_adapter.up.sql`
- `000059_provision_step_attempts.up.sql`
- `000063_ssh_host_key_pin.up.sql`
- `000066_account_provider_observation.up.sql`
- `000067_installer_orchestration.up.sql`
- `000068_installer_selection.up.sql`
- `000069_installer_generations.up.sql`
- `000070_database_deployment_contract.up.sql`
- `000071_xui_panel_deployments.up.sql`
- `000072_deployment_host.up.sql`
- `000073_postinstall_generation.up.sql`
- `000094_account_soft_delete.up.sql`
- `000096_remove_account_login_browser_legacy.up.sql`
- `000097_provider_snapshot_canonical.up.sql`
- `000099_account_proxy_pool.up.sql`
- `000100_provider_capacity_observations.up.sql`
- `000101_vultr_console_capacity_credentials.up.sql`

**Tests:**
- `internal/app/account_state_test.go`
- `internal/app/capacity_guard_test.go`
- `internal/app/proxy_provider_test.go`
- `internal/droplets/compute_stub_test.go`
- `internal/droplets/executor_test.go`
- `internal/droplets/postgres_e2e_test.go`
- `internal/droplets/reconcile_test.go`
- `internal/droplets/safety_characterization_test.go`
- `internal/providers/digitalocean/characterization_test.go`
- `internal/providers/digitalocean/client_test.go`
- `internal/providers/digitalocean/driver_v2_test.go`
- `internal/providers/digitalocean/errors_parse_test.go`
- `internal/providers/digitalocean/errors_test.go`
- `internal/providers/digitalocean/pagination_regression_test.go`
- `internal/providers/provider_v2_test.go`
- `internal/providers/vultr/client_test.go`
- `internal/providers/vultr/driver_test.go`
- `internal/providers/vultr/mutation_driver_test.go`
- `internal/providers/vultr/read_driver_test.go`
- `internal/providers/vultrconsole/browser_test.go`

### ACC-001 — accounts

Accounts retain independent provider, proxy, identity and lifecycle state.

**Why:** Avoid cross-account coupling and shared state regressions.

**Code:**
- `internal/accounts/cell.go`
- `internal/accounts/cell_test.go`
- `internal/accounts/context.go`
- `internal/accounts/context_test.go`
- `internal/accounts/identity.go`
- `internal/accounts/identity_test.go`
- `internal/adminapi/accounts_manage.go`
- `internal/app/account_state.go`

**Database:**
- `SET`
- `account_network_identities`
- `account_proxy_pool`
- `accounts`
- `droplets`
- `network_profiles`
- `provider_capacity_observations`
- `provider_snapshots`
- `proxies`
- `resources`

**Migrations:**
- `000001_core.up.sql`
- `000004_proxy_health.up.sql`
- `000006_provider_registry.up.sql`
- `000007_droplet_lifecycle.up.sql`
- `000015_lifecycle_expiration.up.sql`
- `000023_account_automation.up.sql`
- `000024_account_server_choices.up.sql`
- `000026_proxy_health_successes.up.sql`
- `000029_account_lifecycle_policy.up.sql`
- `000030_replacement_lifecycle.up.sql`
- `000031_account_runtime_status.up.sql`
- `000034_account_desired_server_count.up.sql`
- `000036_account_any_region_fallback.up.sql`
- `000041_account_network_identity.up.sql`
- `000042_randomized_deployment_choices.up.sql`
- `000043_build_spacing.up.sql`
- `000044_build_spacing_range.up.sql`
- `000046_digitalocean_login_credentials.up.sql`
- `000047_build_spacing_max_default.up.sql`
- `000048_sticky_proxy_identity.up.sql`
- `000051_account_browser_runtime.up.sql`
- `000054_browser_login_state.up.sql`
- `000055_login_challenge_state.up.sql`
- `000056_proxy_adapter.up.sql`
- `000066_account_provider_observation.up.sql`
- `000094_account_soft_delete.up.sql`
- `000096_remove_account_login_browser_legacy.up.sql`
- `000097_provider_snapshot_canonical.up.sql`
- `000099_account_proxy_pool.up.sql`
- `000100_provider_capacity_observations.up.sql`
- `000101_vultr_console_capacity_credentials.up.sql`

**Tests:**
- `internal/accounts/cell_test.go`
- `internal/accounts/context_test.go`
- `internal/accounts/identity_test.go`
- `internal/app/account_state_test.go`

### NET-001 — proxy_network

Required account proxy egress is isolated and fail-closed.

**Why:** A failed required proxy must not silently leak direct provider traffic.

**Code:**
- `internal/app/proxy_pool.go`
- `internal/app/sticky_proxy.go`
- `internal/network/client.go`
- `internal/network/client_test.go`
- `internal/network/egress_guard.go`
- `internal/network/egress_guard_test.go`
- `internal/network/gate.go`
- `internal/network/gate_test.go`
- `internal/network/guard.go`
- `internal/network/guard_test.go`
- `internal/network/health.go`
- `internal/network/health_state.go`
- `internal/network/health_state_test.go`
- `internal/network/health_store.go`
- `internal/network/health_test.go`
- `internal/network/ipv4.go`
- `internal/network/ipv4_test.go`
- `internal/network/isolation.go`
- `internal/network/isolation_test.go`
- `internal/network/leakcheck.go`
- `internal/network/leakcheck_test.go`
- `internal/network/metadata.go`
- `internal/network/metadata_test.go`
- `internal/network/model.go`
- `internal/network/monitor.go`
- `internal/network/privacy.go`
- `internal/network/privacy_test.go`
- `internal/network/profiles.go`
- `internal/network/profiles_test.go`
- `internal/network/proxy_failure_test.go`
- `internal/network/proxy_gateway.go`
- `internal/network/proxy_gateway_test.go`
- `internal/network/proxy_probe_test.go`
- `internal/network/rotating_test.go`
- `internal/network/scheduler.go`
- `internal/network/scheduler_test.go`

**Database:**
- `SET`
- `account_network_identities`
- `account_proxy_pool`
- `accounts`
- `network_profiles`
- `proxies`

**Migrations:**
- `000001_core.up.sql`
- `000004_proxy_health.up.sql`
- `000023_account_automation.up.sql`
- `000024_account_server_choices.up.sql`
- `000026_proxy_health_successes.up.sql`
- `000029_account_lifecycle_policy.up.sql`
- `000031_account_runtime_status.up.sql`
- `000034_account_desired_server_count.up.sql`
- `000036_account_any_region_fallback.up.sql`
- `000041_account_network_identity.up.sql`
- `000042_randomized_deployment_choices.up.sql`
- `000043_build_spacing.up.sql`
- `000044_build_spacing_range.up.sql`
- `000046_digitalocean_login_credentials.up.sql`
- `000047_build_spacing_max_default.up.sql`
- `000048_sticky_proxy_identity.up.sql`
- `000051_account_browser_runtime.up.sql`
- `000054_browser_login_state.up.sql`
- `000055_login_challenge_state.up.sql`
- `000056_proxy_adapter.up.sql`
- `000066_account_provider_observation.up.sql`
- `000094_account_soft_delete.up.sql`
- `000096_remove_account_login_browser_legacy.up.sql`
- `000099_account_proxy_pool.up.sql`
- `000101_vultr_console_capacity_credentials.up.sql`

**Tests:**
- `internal/network/client_test.go`
- `internal/network/egress_guard_test.go`
- `internal/network/gate_test.go`
- `internal/network/guard_test.go`
- `internal/network/health_state_test.go`
- `internal/network/health_test.go`
- `internal/network/ipv4_test.go`
- `internal/network/isolation_test.go`
- `internal/network/leakcheck_test.go`
- `internal/network/metadata_test.go`
- `internal/network/privacy_test.go`
- `internal/network/profiles_test.go`
- `internal/network/proxy_failure_test.go`
- `internal/network/proxy_gateway_test.go`
- `internal/network/proxy_probe_test.go`
- `internal/network/rotating_test.go`
- `internal/network/scheduler_test.go`

### LIFE-001 — lifecycle

Server/deployment work is a reconciled observable state machine with one coherent execution owner.

**Why:** Avoid duplicate installers, competing workers and stale-state races.

**Code:**
- `cmd/worker/main.go`
- `internal/app/lifecycle.go`
- `internal/droplets/compute_stub_test.go`
- `internal/droplets/executor.go`
- `internal/droplets/executor_test.go`
- `internal/droplets/factory.go`
- `internal/droplets/lifecycle.go`
- `internal/droplets/lifecycle_engine.go`
- `internal/droplets/model.go`
- `internal/droplets/postgres_e2e_test.go`
- `internal/droplets/reconcile.go`
- `internal/droplets/reconcile_test.go`
- `internal/droplets/safety_characterization_test.go`

**Database:**
- `accounts`
- `deployment_events`
- `deployment_profiles`
- `deployments`
- `droplets`
- `lifecycle_events`
- `network_profiles`
- `panel_instances`
- `provider`
- `resources`
- `schedules`

**Migrations:**
- `000001_core.up.sql`
- `000006_provider_registry.up.sql`
- `000007_droplet_lifecycle.up.sql`
- `000013_deployments.up.sql`
- `000014_deployment_profiles.up.sql`
- `000015_lifecycle_expiration.up.sql`
- `000019_schedules.up.sql`
- `000020_concurrency_hardening.up.sql`
- `000023_account_automation.up.sql`
- `000024_account_server_choices.up.sql`
- `000029_account_lifecycle_policy.up.sql`
- `000030_replacement_lifecycle.up.sql`
- `000031_account_runtime_status.up.sql`
- `000034_account_desired_server_count.up.sql`
- `000036_account_any_region_fallback.up.sql`
- `000042_randomized_deployment_choices.up.sql`
- `000043_build_spacing.up.sql`
- `000044_build_spacing_range.up.sql`
- `000046_digitalocean_login_credentials.up.sql`
- `000047_build_spacing_max_default.up.sql`
- `000051_account_browser_runtime.up.sql`
- `000054_browser_login_state.up.sql`
- `000055_login_challenge_state.up.sql`
- `000066_account_provider_observation.up.sql`
- `000069_installer_generations.up.sql`
- `000072_deployment_host.up.sql`
- `000073_postinstall_generation.up.sql`
- `000074_panel_driver_foundation.up.sql`
- `000094_account_soft_delete.up.sql`
- `000096_remove_account_login_browser_legacy.up.sql`
- `000101_vultr_console_capacity_credentials.up.sql`

**Tests:**
- `internal/droplets/compute_stub_test.go`
- `internal/droplets/executor_test.go`
- `internal/droplets/postgres_e2e_test.go`
- `internal/droplets/reconcile_test.go`
- `internal/droplets/safety_characterization_test.go`

### DO-001 — digitalocean

DigitalOcean behavior remains functional while generic/provider-neutral architecture evolves.

**Code:**
- `internal/providers/digitalocean/account.go`
- `internal/providers/digitalocean/catalog.go`
- `internal/providers/digitalocean/characterization_test.go`
- `internal/providers/digitalocean/client.go`
- `internal/providers/digitalocean/client_test.go`
- `internal/providers/digitalocean/discovery.go`
- `internal/providers/digitalocean/driver_v2.go`
- `internal/providers/digitalocean/driver_v2_test.go`
- `internal/providers/digitalocean/errors.go`
- `internal/providers/digitalocean/errors_parse_test.go`
- `internal/providers/digitalocean/errors_test.go`
- `internal/providers/digitalocean/mutations.go`
- `internal/providers/digitalocean/pagination.go`
- `internal/providers/digitalocean/pagination_regression_test.go`
- `internal/providers/digitalocean/resources.go`
- `internal/providers/digitalocean/types.go`

**Tests:**
- `internal/providers/digitalocean/characterization_test.go`
- `internal/providers/digitalocean/client_test.go`
- `internal/providers/digitalocean/driver_v2_test.go`
- `internal/providers/digitalocean/errors_parse_test.go`
- `internal/providers/digitalocean/errors_test.go`
- `internal/providers/digitalocean/pagination_regression_test.go`

### VULTR-001 — vultr

Vultr uses provider-specific read/mutation/catalog/capacity semantics behind common provider contracts.

**Code:**
- `internal/providers/vultr/client.go`
- `internal/providers/vultr/client_test.go`
- `internal/providers/vultr/driver.go`
- `internal/providers/vultr/driver_test.go`
- `internal/providers/vultr/mutation.go`
- `internal/providers/vultr/mutation_driver.go`
- `internal/providers/vultr/mutation_driver_test.go`
- `internal/providers/vultr/read.go`
- `internal/providers/vultr/read_driver.go`
- `internal/providers/vultr/read_driver_test.go`
- `internal/providers/vultr/types.go`

**Tests:**
- `internal/providers/vultr/client_test.go`
- `internal/providers/vultr/driver_test.go`
- `internal/providers/vultr/mutation_driver_test.go`
- `internal/providers/vultr/read_driver_test.go`

### VULTR-002 — vultr_capacity

Determine Vultr capacity accurately; browser-only interactive capacity observation stays isolated from generic provider core.

**Code:**
- `cmd/vultr-capacity-probe/main.go`
- `internal/adminapi/server.go`
- `internal/providers/vultrconsole/browser.go`
- `internal/providers/vultrconsole/browser_test.go`
- `internal/providers/vultrconsole/proxy_bridge.go`

**Database:**
- `SET`
- `account_proxy_pool`
- `accounts`
- `provider_capacity_observations`
- `proxies`

**Migrations:**
- `000001_core.up.sql`
- `000004_proxy_health.up.sql`
- `000023_account_automation.up.sql`
- `000024_account_server_choices.up.sql`
- `000026_proxy_health_successes.up.sql`
- `000029_account_lifecycle_policy.up.sql`
- `000031_account_runtime_status.up.sql`
- `000034_account_desired_server_count.up.sql`
- `000036_account_any_region_fallback.up.sql`
- `000042_randomized_deployment_choices.up.sql`
- `000043_build_spacing.up.sql`
- `000044_build_spacing_range.up.sql`
- `000046_digitalocean_login_credentials.up.sql`
- `000047_build_spacing_max_default.up.sql`
- `000051_account_browser_runtime.up.sql`
- `000054_browser_login_state.up.sql`
- `000055_login_challenge_state.up.sql`
- `000056_proxy_adapter.up.sql`
- `000066_account_provider_observation.up.sql`
- `000094_account_soft_delete.up.sql`
- `000096_remove_account_login_browser_legacy.up.sql`
- `000099_account_proxy_pool.up.sql`
- `000100_provider_capacity_observations.up.sql`
- `000101_vultr_console_capacity_credentials.up.sql`

**Tests:**
- `internal/adminapi/server_test.go`
- `internal/providers/vultrconsole/browser_test.go`

### VULTR-003 — vultr_console

Cloudflare/CAPTCHA/2FA is solved manually through an interactive console/noVNC session; authorized state may then be reused.

**Code:**
- `internal/adminapi/server.go`
- `internal/providers/vultrconsole/browser.go`
- `internal/providers/vultrconsole/browser_test.go`
- `internal/providers/vultrconsole/proxy_bridge.go`

**Database:**
- `accounts`

**Migrations:**
- `000001_core.up.sql`
- `000023_account_automation.up.sql`
- `000024_account_server_choices.up.sql`
- `000029_account_lifecycle_policy.up.sql`
- `000031_account_runtime_status.up.sql`
- `000034_account_desired_server_count.up.sql`
- `000036_account_any_region_fallback.up.sql`
- `000042_randomized_deployment_choices.up.sql`
- `000043_build_spacing.up.sql`
- `000044_build_spacing_range.up.sql`
- `000046_digitalocean_login_credentials.up.sql`
- `000047_build_spacing_max_default.up.sql`
- `000051_account_browser_runtime.up.sql`
- `000054_browser_login_state.up.sql`
- `000055_login_challenge_state.up.sql`
- `000066_account_provider_observation.up.sql`
- `000094_account_soft_delete.up.sql`
- `000096_remove_account_login_browser_legacy.up.sql`
- `000101_vultr_console_capacity_credentials.up.sql`

**Tests:**
- `internal/adminapi/server_test.go`
- `internal/providers/vultrconsole/browser_test.go`

### PANEL-001 — sanaei

Sanaei/x-ui is a modular panel integration; live reads/writes use Sanaei API where implemented.

**Code:**
- `internal/panels/sanaei/api.go`
- `internal/panels/sanaei/api_error_test.go`
- `internal/panels/sanaei/auth.go`
- `internal/panels/sanaei/auth_test.go`
- `internal/panels/sanaei/client_mutation.go`
- `internal/panels/sanaei/clients.go`
- `internal/panels/sanaei/clients_test.go`
- `internal/panels/sanaei/database.go`
- `internal/panels/sanaei/direct_session.go`
- `internal/panels/sanaei/driver.go`
- `internal/panels/sanaei/driver_test.go`
- `internal/panels/sanaei/inbound_get.go`
- `internal/panels/sanaei/inbounds.go`
- `internal/panels/sanaei/installer.go`
- `internal/panels/sanaei/installer_adapter.go`
- `internal/panels/sanaei/installer_adapter_test.go`
- `internal/panels/sanaei/inventory.go`
- `internal/panels/sanaei/inventory_test.go`
- `internal/panels/sanaei/manager.go`
- `internal/panels/sanaei/mutation.go`
- `internal/panels/sanaei/mutation_test.go`
- `internal/panels/sanaei/panel_configurer.go`
- `internal/panels/sanaei/panel_configurer_test.go`
- `internal/panels/sanaei/panel_page.go`
- `internal/panels/sanaei/panel_session.go`
- `internal/panels/sanaei/panel_session_index_test.go`
- `internal/panels/sanaei/panel_session_test.go`
- `internal/panels/sanaei/postflight/payload.go`
- `internal/panels/sanaei/postflight/sniffing.go`
- `internal/panels/sanaei/postflight/validator.go`
- `internal/panels/sanaei/postflight/validator_test.go`
- `internal/panels/sanaei/raw_list.go`
- `internal/panels/sanaei/reality_scan.go`
- `internal/panels/sanaei/realityconfig/builder.go`
- `internal/panels/sanaei/realityconfig/builder_test.go`
- `internal/panels/sanaei/resilient.go`
- `internal/panels/sanaei/resilient_test.go`
- `internal/panels/sanaei/runtime.go`
- `internal/panels/sanaei/runtime_manager.go`
- `internal/panels/sanaei/runtime_manager_test.go`
- `internal/panels/sanaei/runtime_test.go`
- `internal/panels/sanaei/session_executor.go`
- `internal/panels/sanaei/session_executor_test.go`
- `internal/panels/sanaei/ssh_session_executor.go`
- `internal/panels/sanaei/ssh_session_executor_v2.go`
- `internal/panels/sanaei/ssh_session_executor_v2_post.go`
- `internal/panels/sanaei/store.go`
- `internal/panels/sanaei/template.go`
- `internal/panels/sanaei/template_test.go`
- `internal/panels/sanaei/traffic.go`
- `internal/panels/sanaei/traffic_adapter.go`

**Database:**
- `SET`
- `command`
- `deployments`
- `panel_instances`
- `the`
- `xui_clients`
- `xui_panel_deployments`

**Migrations:**
- `000011_xui_clients.up.sql`
- `000013_deployments.up.sql`
- `000014_deployment_profiles.up.sql`
- `000020_concurrency_hardening.up.sql`
- `000069_installer_generations.up.sql`
- `000071_xui_panel_deployments.up.sql`
- `000072_deployment_host.up.sql`
- `000073_postinstall_generation.up.sql`
- `000074_panel_driver_foundation.up.sql`

**Tests:**
- `internal/panels/sanaei/api_error_test.go`
- `internal/panels/sanaei/auth_test.go`
- `internal/panels/sanaei/clients_test.go`
- `internal/panels/sanaei/driver_test.go`
- `internal/panels/sanaei/installer_adapter_test.go`
- `internal/panels/sanaei/inventory_test.go`
- `internal/panels/sanaei/mutation_test.go`
- `internal/panels/sanaei/panel_configurer_test.go`
- `internal/panels/sanaei/panel_session_index_test.go`
- `internal/panels/sanaei/panel_session_test.go`
- `internal/panels/sanaei/postflight/validator_test.go`
- `internal/panels/sanaei/realityconfig/builder_test.go`
- `internal/panels/sanaei/resilient_test.go`
- `internal/panels/sanaei/runtime_manager_test.go`
- `internal/panels/sanaei/runtime_test.go`
- `internal/panels/sanaei/session_executor_test.go`
- `internal/panels/sanaei/template_test.go`

### OUT-001 — output

Output is fast/live and does not use SSH scraping as its live data plane.

**Code:**
- `internal/adminapi/output.go`
- `internal/adminapi/output_snapshot.go`
- `internal/panels/sanaei/runtime_manager.go`

**Database:**
- `SET`
- `accounts`
- `current_output_uris`
- `deployments`
- `droplets`
- `excluded`
- `inbound_export_metadata`
- `inbound_structural_snapshots`
- `output_config_snapshots`
- `output_share_tokens`
- `panel_instances`

**Migrations:**
- `000001_core.up.sql`
- `000007_droplet_lifecycle.up.sql`
- `000013_deployments.up.sql`
- `000014_deployment_profiles.up.sql`
- `000015_lifecycle_expiration.up.sql`
- `000020_concurrency_hardening.up.sql`
- `000023_account_automation.up.sql`
- `000024_account_server_choices.up.sql`
- `000029_account_lifecycle_policy.up.sql`
- `000030_replacement_lifecycle.up.sql`
- `000031_account_runtime_status.up.sql`
- `000034_account_desired_server_count.up.sql`
- `000036_account_any_region_fallback.up.sql`
- `000042_randomized_deployment_choices.up.sql`
- `000043_build_spacing.up.sql`
- `000044_build_spacing_range.up.sql`
- `000046_digitalocean_login_credentials.up.sql`
- `000047_build_spacing_max_default.up.sql`
- `000051_account_browser_runtime.up.sql`
- `000054_browser_login_state.up.sql`
- `000055_login_challenge_state.up.sql`
- `000066_account_provider_observation.up.sql`
- `000069_installer_generations.up.sql`
- `000072_deployment_host.up.sql`
- `000073_postinstall_generation.up.sql`
- `000074_panel_driver_foundation.up.sql`
- `000087_inbound_export_metadata.up.sql`
- `000090_output_snapshots.up.sql`
- `000091_inbound_structural_snapshots.up.sql`
- `000092_output_visibility.up.sql`
- `000094_account_soft_delete.up.sql`
- `000096_remove_account_login_browser_legacy.up.sql`
- `000101_vultr_console_capacity_credentials.up.sql`

**Tests:**
- `internal/panels/sanaei/runtime_manager_test.go`

### OUT-002 — output_expiry

Direct Output and share links apply canonical exact visibility/expiry semantics so invalid configs disappear on time.

**Code:**
- `internal/adminapi/output.go`
- `internal/adminapi/output_snapshot.go`

**Database:**
- `SET`
- `accounts`
- `current_output_uris`
- `deployments`
- `droplets`
- `excluded`
- `inbound_export_metadata`
- `inbound_structural_snapshots`
- `output_config_snapshots`
- `output_share_tokens`
- `panel_instances`

**Migrations:**
- `000001_core.up.sql`
- `000007_droplet_lifecycle.up.sql`
- `000013_deployments.up.sql`
- `000014_deployment_profiles.up.sql`
- `000015_lifecycle_expiration.up.sql`
- `000020_concurrency_hardening.up.sql`
- `000023_account_automation.up.sql`
- `000024_account_server_choices.up.sql`
- `000029_account_lifecycle_policy.up.sql`
- `000030_replacement_lifecycle.up.sql`
- `000031_account_runtime_status.up.sql`
- `000034_account_desired_server_count.up.sql`
- `000036_account_any_region_fallback.up.sql`
- `000042_randomized_deployment_choices.up.sql`
- `000043_build_spacing.up.sql`
- `000044_build_spacing_range.up.sql`
- `000046_digitalocean_login_credentials.up.sql`
- `000047_build_spacing_max_default.up.sql`
- `000051_account_browser_runtime.up.sql`
- `000054_browser_login_state.up.sql`
- `000055_login_challenge_state.up.sql`
- `000066_account_provider_observation.up.sql`
- `000069_installer_generations.up.sql`
- `000072_deployment_host.up.sql`
- `000073_postinstall_generation.up.sql`
- `000074_panel_driver_foundation.up.sql`
- `000087_inbound_export_metadata.up.sql`
- `000090_output_snapshots.up.sql`
- `000091_inbound_structural_snapshots.up.sql`
- `000092_output_visibility.up.sql`
- `000094_account_soft_delete.up.sql`
- `000096_remove_account_login_browser_legacy.up.sql`
- `000101_vultr_console_capacity_credentials.up.sql`

### REAL-001 — reality

Reality/VLESS ports, SNI, target users, quota, lifetime and device limit remain policy-driven.

**Code:**
- `internal/panels/globalreality/service.go`
- `internal/panels/policy/allocator.go`
- `internal/panels/policy/allocator_test.go`
- `internal/panels/policy/expand.go`
- `internal/panels/policy/expand_test.go`
- `internal/panels/policy/model.go`
- `internal/panels/policy/normalize.go`
- `internal/panels/policy/store.go`
- `internal/panels/policy/validate.go`
- `internal/panels/policy/validate_test.go`

**Database:**
- `SET`
- `deployments`
- `global_config_policies`
- `panel_inbound_inventory`
- `panel_inbound_policies`
- `panel_instances`
- `reality_target_selections`

**Migrations:**
- `000013_deployments.up.sql`
- `000014_deployment_profiles.up.sql`
- `000020_concurrency_hardening.up.sql`
- `000069_installer_generations.up.sql`
- `000072_deployment_host.up.sql`
- `000073_postinstall_generation.up.sql`
- `000074_panel_driver_foundation.up.sql`
- `000075_panel_inbound_inventory.up.sql`
- `000076_panel_inbound_policies.up.sql`
- `000077_reality_target_observations.up.sql`
- `000079_reality_scan_selection.up.sql`
- `000084_config_policy_controls.up.sql`
- `000085_global_config_policy.up.sql`

**Tests:**
- `internal/panels/policy/allocator_test.go`
- `internal/panels/policy/expand_test.go`
- `internal/panels/policy/validate_test.go`

### RES-001 — residential

Residential routing is separate from account/provider control-plane proxy assignment.

**Code:**
- `internal/adminapi/residential.go`
- `internal/panels/residentialsync/service.go`

**Database:**
- `deployments`
- `panel_instances`
- `proxies`
- `residential_proxies`
- `xui_panel_deployments`

**Migrations:**
- `000001_core.up.sql`
- `000004_proxy_health.up.sql`
- `000013_deployments.up.sql`
- `000014_deployment_profiles.up.sql`
- `000020_concurrency_hardening.up.sql`
- `000026_proxy_health_successes.up.sql`
- `000056_proxy_adapter.up.sql`
- `000069_installer_generations.up.sql`
- `000071_xui_panel_deployments.up.sql`
- `000072_deployment_host.up.sql`
- `000073_postinstall_generation.up.sql`
- `000074_panel_driver_foundation.up.sql`
- `000086_residential_proxies.up.sql`

### UI-001 — frontend

Admin modules remain operational/mobile-friendly and preserve working context; interactive challenges provide visible browser feedback.

### OPS-001 — production

Never infer production revision from service health alone; verify Git/build/runtime/artifact hashes.

**Code:**
- `internal/buildinfo/buildinfo.go`

### KNOW-001 — continuity

Fresh sessions recover intent, architecture, source lines, semantic relationships, schema, tests and runtime from commit-pinned project knowledge.

