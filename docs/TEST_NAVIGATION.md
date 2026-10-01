# Test / Verification Map

Indexed **121 test files** containing **251 Test* functions**. Full machine-readable map: `docs/TEST_MAP.json`.

Use the narrowest relevant tests first, then broader verification appropriate to blast radius. A passing unit test does not replace runtime smoke verification for deployed behavior.

## `providers_common`
- Test files: **12**; Test functions: **34**
- Focused command: `go test ./internal/providers/...`
- Files: `internal/providers/digitalocean/characterization_test.go`, `internal/providers/digitalocean/client_test.go`, `internal/providers/digitalocean/driver_v2_test.go`, `internal/providers/digitalocean/errors_parse_test.go`, `internal/providers/digitalocean/errors_test.go`, `internal/providers/digitalocean/pagination_regression_test.go`, `internal/providers/provider_v2_test.go`, `internal/providers/vultr/client_test.go`, `internal/providers/vultr/driver_test.go`, `internal/providers/vultr/mutation_driver_test.go`, `internal/providers/vultr/read_driver_test.go`, `internal/providers/vultrconsole/browser_test.go`

## `digitalocean`
- Test files: **6**; Test functions: **18**
- Focused command: `go test ./internal/providers/digitalocean/...`
- Files: `internal/providers/digitalocean/characterization_test.go`, `internal/providers/digitalocean/client_test.go`, `internal/providers/digitalocean/driver_v2_test.go`, `internal/providers/digitalocean/errors_parse_test.go`, `internal/providers/digitalocean/errors_test.go`, `internal/providers/digitalocean/pagination_regression_test.go`

## `vultr`
- Test files: **5**; Test functions: **11**
- Focused command: `go test ./internal/providers/vultr/... ./internal/providers/vultrconsole/...`
- Files: `internal/providers/vultr/client_test.go`, `internal/providers/vultr/driver_test.go`, `internal/providers/vultr/mutation_driver_test.go`, `internal/providers/vultr/read_driver_test.go`, `internal/providers/vultrconsole/browser_test.go`

## `accounts`
- Test files: **3**; Test functions: **3**
- Focused command: `go test ./internal/accounts/... ./internal/adminapi/...`
- Files: `internal/accounts/cell_test.go`, `internal/accounts/context_test.go`, `internal/accounts/identity_test.go`

## `network_proxy`
- Test files: **17**; Test functions: **30**
- Focused command: `go test ./internal/network/...`
- Files: `internal/network/client_test.go`, `internal/network/egress_guard_test.go`, `internal/network/gate_test.go`, `internal/network/guard_test.go`, `internal/network/health_state_test.go`, `internal/network/health_test.go`, `internal/network/ipv4_test.go`, `internal/network/isolation_test.go`, `internal/network/leakcheck_test.go`, `internal/network/metadata_test.go`, `internal/network/privacy_test.go`, `internal/network/profiles_test.go`, `internal/network/proxy_failure_test.go`, `internal/network/proxy_gateway_test.go`, `internal/network/proxy_probe_test.go`, `internal/network/rotating_test.go`, `internal/network/scheduler_test.go`

## `compute_lifecycle`
- Test files: **5**; Test functions: **7**
- Focused command: `go test ./internal/droplets/...`
- Files: `internal/droplets/compute_stub_test.go`, `internal/droplets/executor_test.go`, `internal/droplets/postgres_e2e_test.go`, `internal/droplets/reconcile_test.go`, `internal/droplets/safety_characterization_test.go`

## `sanaei`
- Test files: **17**; Test functions: **44**
- Focused command: `go test ./internal/panels/sanaei/...`
- Files: `internal/panels/sanaei/api_error_test.go`, `internal/panels/sanaei/auth_test.go`, `internal/panels/sanaei/clients_test.go`, `internal/panels/sanaei/driver_test.go`, `internal/panels/sanaei/installer_adapter_test.go`, `internal/panels/sanaei/inventory_test.go`, `internal/panels/sanaei/mutation_test.go`, `internal/panels/sanaei/panel_configurer_test.go`, `internal/panels/sanaei/panel_session_index_test.go`, `internal/panels/sanaei/panel_session_test.go`, `internal/panels/sanaei/postflight/validator_test.go`, `internal/panels/sanaei/realityconfig/builder_test.go`, `internal/panels/sanaei/resilient_test.go`, `internal/panels/sanaei/runtime_manager_test.go`, `internal/panels/sanaei/runtime_test.go`, `internal/panels/sanaei/session_executor_test.go`, `internal/panels/sanaei/template_test.go`

## `reality_policy`
- Test files: **4**; Test functions: **6**
- Focused command: `go test ./internal/panels/policy/... ./internal/panels/realityconfig/... ./internal/panels/realitycontract/... ./internal/panels/globalreality/...`
- Files: `internal/panels/policy/allocator_test.go`, `internal/panels/policy/expand_test.go`, `internal/panels/policy/validate_test.go`, `internal/panels/realitycontract/contract_test.go`

## `panel_lifecycle`
- Test files: **5**; Test functions: **11**
- Focused command: `go test ./internal/panels/create/... ./internal/panels/update/... ./internal/panels/desired/... ./internal/panels/readyworker/...`
- Files: `internal/panels/create/orchestrator_test.go`, `internal/panels/desired/lifecycle_test.go`, `internal/panels/readyworker/adapter_test.go`, `internal/panels/readyworker/worker_test.go`, `internal/panels/update/orchestrator_test.go`

## `output`
- Test files: **1**; Test functions: **2**
- Focused command: `go test ./internal/adminapi/... ./internal/panels/sanaei/...`
- Files: `internal/adminapi/output_expiry_test.go`

## `adminapi`
- Test files: **3**; Test functions: **5**
- Focused command: `go test ./internal/adminapi/...`
- Files: `internal/adminapi/config_expr_test.go`, `internal/adminapi/output_expiry_test.go`, `internal/adminapi/server_test.go`

## `auth`
- Test files: **1**; Test functions: **3**
- Focused command: `go test ./internal/auth/...`
- Files: `internal/auth/service_test.go`

## `jobs`
- Test files: **1**; Test functions: **1**
- Focused command: `go test ./internal/jobs/...`
- Files: `internal/jobs/operation_test.go`

## Verification tiers
- Focused: mapped subsystem/package tests.
- Broad: `go test ./internal/...` when cross-cutting behavior changed.
- Build: `go build ./cmd/api ./cmd/worker` before production deployment of Go changes.
- Runtime: verify API/worker service health, migrations when relevant, then exercise the real endpoint/UI/provider flow.
