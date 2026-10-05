# UpCloud provider continuation — 2026-10-05

Runtime API/worker and static source: cb131196167337612de85fc44d3005ffa9eae7ee, clean detached build. Canonical branch remains checkpoint/final-e2e-20260929. Read UPCLOUD_ACCEPTANCE_20261005.json and current source; this is the new frontier.

## User flow and shared behavior

Accounts → New → UpCloud → API token (ucat_…), choose the same direct/proxy chain, validate, then choose regions/plans/Ubuntu/lifetime/desired count/build spacing. UpCloud is a registered ready provider; provider/token/primary-proxy edits invalidate stale preview. No UpCloud credentials were supplied or discovered: real cloud lifecycle acceptance remains pending until the user adds an account through the panel.

The shared scheduler, encrypted per-deployment SSH identity, kernel memory guard, Sanaei/Reality activation, residential synchronization, future permanent-profile enrollment, output, expiry, cleanup jobs and proxy generation checks are reused. Raw SSH public keys are passed inline because UpCloud does not use the other providers' SSH key objects. Provisioning starts with public/utility IPv4, provider firewall off to use the existing common guest provisioning policy, metadata enabled, no password delivery, no remote VNC and no automatic backups.

## Provider-specific safety and evidence

- API https://api.upcloud.com/1.3 uses bearer tokens exclusively through the account's supplied transport. Redirects are refused; timeouts and response sizes bounded; error bodies do not leak credentials.
- Catalog uses public zones, CPU plans and supported public Ubuntu templates. Unknown prices remain unavailable.
- Capacity uses account resource limits and account/resource-usage. Memory is MiB, storage is GiB per current official OpenAPI. Selected plans determine conservative remaining slots across CPU/RAM/IPv4/storage and optional plan quotas. This is a resource budget, not an exact account server-count quota. Incomplete required evidence blocks fresh capacity acceptance.
- Unique owner/identity labels on server and cloned root storage allow adoption after an ambiguous create. No automatic create POST retry.
- Migration 153 stores immutable per-server cleanup manifests before stop/delete. Stop is asynchronous. Server deletion detaches disks; only recorded, freshly ownership-checked, unattached owned disks are deleted, including owned disks detached before cleanup. Separate backups and foreign/manual disks are preserved. A pending manifest makes GetServer return deleting until all recorded disks are confirmed absent.
- Account disabled/deleting blocks create. Only the exact UpCloud stop path for a persisted pending manifest is allowed through the mutation fence. Completion atomically retires storage registry rows, allowing disabled-account cleanup to finish.
- Billing presents available credit and month-to-date usage in actual reported currency. Invoice settlement status stays unknown.
- Caveat: external VM deletion before a cleanup manifest exists or out-of-band storage ownership changes still require operator reconciliation; do not erase pending records or imply universal orphan recovery.

## Acceptance and rollout

Full Go suite, real isolated PostgreSQL and race suite, provider contract/fault tests, all four browser scenarios, and production mobile form/read-only authenticated API checks passed. No paid UpCloud resource was created, and no live provider SSH/panel/delete success is claimed.

Two local acceptance-script assumptions caused rollback after otherwise healthy starts: nonexistent global resources route, then nonexistent dirty migration column. Both original logs remain in deploy-attempt-1/2 and both automatic restorations were verified active on prior source 54cbc731. Corrected account-scoped resources and exact native migration checksum checks pass. Final deploy status DEPLOYED_CONFIG_PRESERVED. Subsequent fixes after implementation commit 0f86bbc touch only the standalone acceptance script.

The user has independently published permanent fleet profile 38f9c98e-0acb-4f95-8639-8e65087edce0, state KEPT, no deadline, fast_count=13, fast_share=80, TCP Fast Open=true. This supersedes the earlier handoff's no-active-profile statement. Exact profiles, gates, routing control and endpoint configuration hashes matched before/after. Preserve google@ads + temporary BrowserLeaks, UDP and server-direct IPv4 DNS. Existing fleet/network evidence is historical; this change does not assert a new full fleet or AdMob performance result.

## Rollback and continuation

Evidence/backups: /root/backups/dob-upcloud-20261005. Guarded manual script: /root/backups/dob-upcloud-20261005/rollback.sh. It refuses downgrade while any non-deleted UpCloud account or pending cleanup manifest exists. Clean those using this driver first; do not remove credentials before provider resources are confirmed absent. Restore prior 54cbc731 binaries and app.js; leave additive migration 153 in place. The prior version supports the user's active permanent residential publication, which must remain unchanged. Pre-152 downgrades still require restoring permanent profiles with the appropriate controller.

Next: user adds an UpCloud token via the panel, then perform a bounded real lifecycle canary with account capacity and proxy readiness verified; observe create → SSH → panel/Reality → inherited profile → expiry/delete → owned storage absence. Do not call fixture evidence live provider proof.

Official references inspected: developers.upcloud.com/1.3/ (accounts, plans, servers, storages, IPs, firewall), developers.upcloud.com/api/1.3/upcloud-openapi.json and modern account resource-usage/billing summary schemas.
