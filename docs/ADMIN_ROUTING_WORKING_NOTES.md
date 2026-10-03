# Admin deletion, routing and Output — candidate checkpoint (2026-10-03)

Starting canonical HEAD: 09b63e5016a8a92bce2f27e35ce5ad3e8966a3e4, clean and synchronized with checkpoint/final-e2e-20260929.
Starting runtime: b789a47639f5299e84093024262d799edaf644fb; production migrations through 137.
Evidence directory: /root/backups/dob-admin-routing-20261003.
This checkpoint describes candidate code and tests. Production deployment and acceptance are still pending.

## Implemented
- Residential deletion removes residential membership and preserves a proxy shared by account API connections. Full proxy deletion detaches account assignments while preserving proxy_required; missing proxy fails closed.
- Residential endpoint/credential changes commit atomically. Revision triggers queue routing reconciliation.
- Explicit account deletion persists a worker job, disables scheduling/lifecycle scopes, fences in-flight provider mutations, and retains recovery credentials until owned provider servers/SSH keys are verified absent. Unknown creates remain unresolved until provider evidence resolves them. Operation recovery uses lock-version guards.
- Account data purge removes account children and diagnostics in one transaction. Existing historical soft-deleted accounts are not automatically purged.
- Dashboard health groups are mutually exclusive; stale observations are unverified. Worker heartbeats are emitted. Capacity totals exclude stale/expired/missing inbounds.
- Billing amounts and invoice dates appear on account list/detail. Provider invoice settlement is UNKNOWN: inspected DO/Vultr API models do not prove per-invoice unpaid status. Transport failures preserve stale data and report unavailable.
- Global Reality has Residential and Direct selection flags. The existing total client target is shared, not doubled. Existing UUID/email identities remain unchanged.
- Durable routing membership and plan hash are persisted before network changes. Routing uses actual inbound tags, exact client emails, explicit direct/proxy/blackhole outbounds, and preserves API/security rules.
- Configured-but-unusable residential means BLOCKED; no residential configuration means DIRECT. HTTP residential cannot carry UDP; UDP is explicitly blocked for that class.
- Routing uses shared RuntimeManager mutation locking plus cross-process panel config locking. The template is freshly verified, then the running Xray router is probed. Content-addressed outbound tags include complete configuration and membership, so a lost save/reload response can be reconciled without a duplicate reload. Unchanged plans are verified without republishing membership or restarting.
- Proxy selection is rechecked before/after apply. Output requires the exact selected proxy to remain eligible.
- Output has two class-bound share links; query parameters cannot widen a token. Only fresh current-revision running-core proofs publish class output. Existing ALL tokens remain compatible.
- Delete All Clients & Inbounds persists scope/membership, freezes creation and execution gates, executes bounded worker-only chunks, and verifies each client/inbound absent. Unexpected identities pause it. Retrying resumes the same immutable scope. Historical failed CREATE audit rows are preserved.
- Read inventories reject missing/null arrays; provider pagination is bounded and supports Vultr opaque cursors.

## Authoritative target evidence
Read-only target: 0a1bfc1f-0415-413b-bd92-71b37cea62b4 (recheck lifetime before mutation).
Installed Xray: 26.9.9. The installed binary accepted the synthetic SOCKS/user-routing payload: Configuration OK (installed-xray-config-test-2.log). No service/config change was made by that validation.
Installed v3 OpenAPI proves:
- POST panel/api/xray/update saves the JSON template.
- POST panel/api/server/restartXrayService reloads it.
- POST panel/api/xray/routeTest asks the running core's RoutingService.TestRoute; no traffic is sent. Form fields include ip, port, network, inboundTag, email; result has matched/outboundTag.
- POST panel/api/inbounds/del/{id} deletes an inbound.
Observed inbound tag was in-443-tcp, not inferred from port.

## Validation
Full go test ./..., focused race, JS syntax, and git diff checks passed before the final additional routing-worker integration test. That integration test also passed with race:
- Healthy residential + direct groups.
- Proxy down -> residential blocked, direct preserved.
- Residential removal -> direct.
- Lost save/reload responses reconciled.
- Unchanged plan causes no repeated reload.
Other PostgreSQL/HTTP tests verify disjoint Output, immutable share class, stale proof rejection, shared proxy deletion, account purge isolation, in-flight provider fencing, and durable all-client/inbound cleanup across worker restart with lost responses.
All PostgreSQL tests use dob_bulk_test_20261003 and isolated schemas. Historical migration51's public catalog lookup is adapted only in temporary test copies.

## Pending acceptance and constraints
Run final clean detached full tests/build, deploy migrations 138–142 and binaries/static assets atomically, verify services/API, run synthetic disabled-account/proxy deletion acceptance, then scoped routing canary and gradual activation.
Do not clear the actual production fleet as a test.
The configured residential proxy was DOWN and shared by nine account network profiles. Do not bypass their proxy_required guard. Live provider billing/owned-resource cleanup and actual residential egress cannot be declared accepted without a healthy authorized transport.
Actual quota-exhaustion traffic test remains AFTER these fixes and routing acceptance.
Preserve live policy observed target=2, rate=1, lifetime=10800, quota=0 unless a scoped test journals and restores a deliberate change.
