# Admin deletion, routing and Output — production continuation (2026-10-03)

Authoritative source: /root/projects/Digital-ocean-bot-canonical-e2e, continuation branch checkpoint/final-e2e-20260929.
Production evidence: /root/backups/dob-admin-routing-20261003.
Machine-readable acceptance and exact runtime revisions: docs/ADMIN_ROUTING_ACCEPTANCE.json.
Re-read Production before continuing; the provider fleet is creating/retiring servers and observations change.

## Implemented and deployed
- Residential deletion removes residential membership while preserving a proxy shared by account API connections. Full proxy deletion detaches account references but preserves proxy_required, so missing transport fails closed.
- Residential endpoint and encrypted credentials commit atomically; revision triggers schedule reconciliation.
- Explicit account deletion queues a durable worker job, fences provider mutations, freezes scheduling, and retains credentials until owned servers/SSH keys are verified absent. Ambiguous creates remain unresolved. Final purge removes stored account children, operations, diagnostics and secrets.
- Dashboard account/provider and server health counts are mutually exclusive. Repeated fresh panel routing failures count as broken; retirement/expiry has priority. Stale observations are unverified, not healthy.
- Billing amount/date appears in list and detail. DO and Vultr live observations succeeded. Their inspected API models do not prove per-invoice settlement; UNKNOWN is displayed instead of fabricated unpaid debt.
- Global Reality has Residential/Direct checkboxes, sharing the existing total target. Both enabled preserves the class of each existing UUID/email across deletion, replacement and growth; new identities fill the split.
- Routing membership/plan hash persist before mutation. Actual inbound tags and exact emails are used; API/security rules and unknown JSON fields are retained.
- Configured but unavailable Residential means BLOCKED. No Residential association means DIRECT traffic. A Residential-class identity still does not enter the dedicated Direct subscription.
- Template readback plus running Xray routeTest verifies applied configuration. Content-addressed tags allow lost save/reload response reconciliation without blind repeat. Unchanged plans do not reload.
- Serving and retiring/expired panels have separate serial worker lanes, total concurrency two. Retired-panel timeouts do not block live proof refresh.
- Class-bound Output tokens cannot widen class via query parameters. Publication requires matching intended and effective class, fresh current-revision running proof, and healthy selected Residential proxy where applicable.
- Delete All Clients & Inbounds durably captures scope and membership, freezes creation, deletes bounded worker-only chunks, and freshly verifies absence. Unknown/conflicting identities pause the job. Retry resumes the same scope; historical failed CREATE audit rows remain.

## Production acceptance
- Disabled synthetic account/proxy fixtures passed actual API Residential Delete (shared proxy preserved), Proxy Delete (proxy_required preserved), Account Delete (202 -> worker purge), and list/detail reads. Fixtures and temporary admin sessions were removed.
- One-panel and four-panel canaries proved DIRECT/BLOCKED routes while the actual Residential proxy was down. Client identity/policy hashes were unchanged.
- Initial fleet observation confirmed 38 panels; 15 expired/retiring panels timed out. A later serving panel also timed out despite SSH/x-ui responding; class output correctly remains hidden.
- Three-minute live monitoring: 36 samples, all five then-serving panels verified in every sample; maximum proof age 21.900337 seconds.
- During concurrent Production operations the Residential association disappeared and the shared account proxy recovered. Its nine account references were preserved. This task did not recreate that association. Fresh no-Residential reconciliation and empty Residential output were observed.
- Latest actual class response verification proves disjoint subscriptions and matching intended/effective membership; counts and timestamp are in ADMIN_ROUTING_ACCEPTANCE.json.
- Global policy after isolated tests: enabled, total target 2/inbound, quota 0, lifetime 10800 seconds, device limit 0, rate 1, both classes enabled. Main lifecycle executor retains the previously live setting enabled=true/kill_switch=false/concurrency=1; legacy bulk gate remains closed.
- API/worker are active, builds have vcs.modified=false. Source and continuation remote are synchronized at each deployment checkpoint.

## Actual quota exhaustion acceptance
Run 5523aae1-149d-498f-b8ef-37d39f1414de, panel 0a1bfc1f-0415-413b-bd92-71b37cea62b4, inbound 1.
Completed 2026-10-03T16:03:55.890324Z.
Three owned temporary clients were created, durably updated to 2 MiB / 600 seconds / HWID 3.
One client received 6 MiB through an isolated local Xray tunnel and bounded callback on the control server.
Fresh Sanaei counters: upload 132 bytes, download 6291675 bytes.
Worker deleted the exhausted identity and created exactly one replacement; replacement Output was observed.
All four test identities were subsequently deleted, Output residue is zero, and baseline two-client hash was restored exactly:
6c772736f06d5b4ea41eba9f4adafad7082524de3e97e3e0cfb4c40c4dad30b0.
Journal: two BULK_CREATE, three UPDATE, two BULK_DELETE; all SUCCEEDED/attempt=1.
The local test listener/process/config were removed. This proves quota-triggered lifecycle recovery, not packet-exact cutoff at 2 MiB.

## Tests and limits
Full go test ./..., focused PostgreSQL/race, clean detached full tests/builds, JS syntax and diff checks passed.
Tests include lost save/reload/delete responses, partial cleanup across worker restart, provider pagination ambiguity, account fencing/purge isolation, class stability, strict Output membership and dashboard failure/retirement precedence.
PostgreSQL fixtures use dob_bulk_test_20261003 with isolated schemas. Historical migration51 catalog lookup is adapted only in test copies.
A full Production fleet wipe was not used as a test; durable all-client/inbound cleanup has PostgreSQL/HTTP fault acceptance.
A real provider-populated account was not destroyed as a test; empty fixture purge passed Production, provider cleanup/recovery has integration coverage.
Healthy Residential traffic through the installed Production router remains untested because no Residential association is currently configured. Installed-Xray payload validation and healthy/down/removal HTTP/PostgreSQL routing tests passed.
Next work should address any still-unreachable serving panel through fresh diagnostics, and test healthy Residential egress once an intended Residential configuration exists. Do not restore removed configuration or retry historical failed jobs blindly.
