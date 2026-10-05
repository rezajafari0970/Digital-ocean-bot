# UpCloud capacity and create status — 2026-10-05

## Authority
- Runtime API/worker and app.js/index.html: clean detached build d384c8b054f57c4b24cbea57bc58e05c5317fd04.
- Canonical branch: checkpoint/final-e2e-20260929.
- Migration: 000154_upcloud_create_block.
- Evidence: docs/UPCLOUD_CAPACITY_STATUS_ACCEPTANCE_20261005.json and /root/backups/dob-upcloud-capacity-status-20261005.
- Continue here; do not replay earlier cloud/catalog/routing canaries.

## Finding and implementation
User supplied saved-account screenshots: selected resource budget2, desired5, provider inventory0, conflicting Yes/No/Ready, failed create. Actual deployment evidence is provider permission_denied: UpCloud HTTP 403 (TRIAL_FIREWALL). Successful account/inventory refresh incorrectly erased the semantic create failure; detail and list derived eligibility differently.

Both account views now share fresh-state/count derivation: provider usage, remaining desired target, pending reservations, enabled/runtime/provider states and independent create block. Counts stay unknown when authoritative quota is unknown/stale; confirmed block gives0. Selected-plan resource budgets are exposed separately and never summed. Existing DO/Vultr semantics are covered by the same PostgreSQL list/detail regression matrix. No quota unit change: current OpenAPI explicitly defines storage quota in GiB, superseding obsolete prose claiming MiB.

Migration154 creates account-scoped durable, versioned create blocks and backfills existing actual failed-create evidence; later successful creates/recovery audits exclude obsolete evidence. Typed safe UpCloud403 create rejection writes a block; ordinary GET failures, quota/rate/network errors and other providers do not. Audit write failure cannot undo the block. Scheduler/common capacity and final pre-create checks refuse blocked accounts. Successful provider refresh, preflight and restart cannot remove the independent block. The optional documented trial_mode1 classifies future observations as trial_restricted and prevents initial admission; unknown/absent mode is not treated as proof a previous denial was resolved.

Authenticated admin recovery requires confirmation and current block version, checks known trial evidence and records operator/version audit transactionally. Failed audit/stale version cannot release. It enables existing scheduling policy; it does not immediately bypass admission, alter firewall/ports, change identity, or issue a test create.

## Production proof
Live Upcloud 11 has per-plan resource budgets 2/2/2, provider servers0 and desired5; latest actual create rejected HTTP403/TRIAL_FIREWALL. Durable block survives a successful explicit provider refresh; list/detail show buildable0 consistently. Readable account status ACTIVE is not create permission. Latest read did not expose a positive trial_mode flag; known create rejection remains authoritative until explicit operator recovery. Full paid UpCloud lifecycle remains unverified.

A real explicit Refresh returned HTTP200 in 7.722s, produced a fresh quota snapshot, and retained the same TRIAL_FIREWALL block. Both list and detail returned buildable0/provider_can_create=false/scheduler_can_build=false and the same reason; provider budget2 and per-plan2/2/2 stayed visible. First explicit refresh returned409 at the runtime boundary; its exact underlying reason was not captured. Subsequent read-only identity check was healthy/no collision and retry succeeded. No direct route or admission override was used.

API/worker active, readyz healthy, served static hashes match clean build. Existing account configuration, proxies, routing/gates and permanent residential profile compare unchanged. Both old and new account eligibility tests, full Go suite, race/PostgreSQL/fault/migration and real Chromium mobile/desktop browser checks pass. Acceptance did not create/delete any cloud resources.

## User recovery
Resolve the provider trial/firewall restriction through UpCloud. Then Refresh Account Data and use **Restriction resolved — enable retry** for the recorded block. A new denial recreates the block. If the provider still reports trial_mode1, recovery is rejected. A readable account or fresh quota alone never proves permission to create this deployment.

Do not describe live UpCloud provisioning parity as fully proven while creation is vendor-blocked. Catalog/regions/plans/images/pricing, shared proxy/SSH/scheduler/Sanaei/Reality/residential/expiry/owned cleanup remain implemented and regression-tested, but no complete paid UpCloud live lifecycle has succeeded in this task.

## Rollback
Previous API/worker binaries and app.js/index.html are saved under /root/backups/dob-upcloud-capacity-status-20261005. Migration154 is additive. Its down migration refuses active blocks; do not drop evidence to force downgrade. Older workers cannot enforce the new guard: pause blocked UpCloud accounts before starting them. The deployment rollback script performs that targeted pause and retains block/audit evidence; re-enable only after reinstalling this guard and reviewing provider restriction. Preserve the user's permanent fleet profile and existing network gates.
