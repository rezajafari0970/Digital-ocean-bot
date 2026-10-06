# Development Ready Handoff

## Authority
- Repository: `/root/projects/Digital-ocean-bot-canonical-e2e`
- Branch: `checkpoint/final-e2e-20260929`
- Use the repository and current production-status artifacts as authority; do not rely on unaided verbatim memory.
- `docs/FINAL_KNOWLEDGE_BASELINE.json` fingerprints the deterministic knowledge corpus used for fresh-session retrieval.

## Fresh-session bootstrap
1. Read `docs/FINAL_KNOWLEDGE_BASELINE.json`.
2. Read `docs/CONTINUITY_FINAL_STATUS.json`, `docs/KNOWLEDGE_DRIFT_STATUS.json`, and `docs/PRODUCTION_REVISION_STATUS.json`.
3. For source questions, retrieve from `docs/SOURCE_INTERNALIZATION_BRAIN.json` and `docs/SYMBOL_TOPOLOGY_MEMORY_PACK.json`; inspect live source before mutation.
4. Check `git status`, HEAD, and relevant tests before editing.
5. Continue from the current development frontier rather than replaying memory rehearsal.

## Memory-training disposition
The autonomous spaced-rehearsal service is intentionally disabled. API-runner training is not treated as transfer of memory into the interactive ChatGPT conversation. Historical holdouts remain evidence only and are not rewritten.

## Development gate
Development may resume when the knowledge artifacts exist, their hashes are frozen in the baseline, repository drift is checked at session start, and changes are grounded in live source/tests rather than unsupported recall.

## Latest production continuation (2026-10-03)
- Read `docs/DURABLE_BULK_V3_HANDOFF.md` and `docs/BULK_V3_ACCEPTANCE.json` for the current frontier.
- Durable v3 bulk ten-client canary and cleanup passed; runtime source af1a541799088c6d3c89af675d5f6b02f82cd84c.
- Any following evidence-only commit does not imply binary drift; compare product paths before rebuilding.
- Gates remain closed. Next step is measured chunk-25 acceptance, with bulk policy/expiry lifecycle integration required before fleet enablement.

## Latest ramp checkpoint (2026-10-03)
- Read `docs/BULK_V3_25_ACCEPTANCE.json`: stage 25 is accepted, including complete resource sampling and cleanup.
- Next measured gate is 50 clients. Main/bulk execution gates remain closed; fleet generation is disabled.
- Build on the main disk with a disk-backed GOTMPDIR; /tmp tmpfs exhaustion was observed and resolved for this workflow.

## Latest ramp checkpoint — stage 50 (2026-10-03)
- Read `docs/BULK_V3_50_ACCEPTANCE.json`: one 50-client batch passed create, Output, complete resource sampling and cleanup.
- Next measured gate is 100 clients; current utility is bounded to 10/25/50. Both execution gates and fleet generation remain closed.
- Capacity table timestamp predates this stage; fresh Sanaei readback proves the unchanged one-client baseline.

## Latest ramp checkpoint — stage 100 (2026-10-03)
- Read `docs/BULK_V3_100_ACCEPTANCE.json`: one 100-client batch passed create, Output, complete resource sampling and cleanup.
- Next: prepare stage 250 with durable rate<=100/sec and sufficient cleanup/sampling time; allowlist-only expansion is insufficient.
- Current canary accepts 10/25/50/100. Both gates and fleet generation remain closed. Runtime API/worker remain af1a541.

## Latest scale checkpoint — stage 10000 (2026-10-03)
- Read `docs/BULK_V3_10000_ACCEPTANCE.json` and the final section of `docs/DURABLE_BULK_V3_HANDOFF.md`. The 10000 configured-client stage is accepted and cleanup is complete.
- 50 existing clients were preserved; 9950 test clients reached fresh Sanaei/Output confirmation and durable deletion. 100 CREATE and 100 DELETE jobs are SUCCEEDED, all attempts=1; one committed DELETE was reconciled by explicit fresh read after a verification timeout. Original failure evidence remains.
- Production runtime: ae5ccdbe8d41a77078c12c371297bc6543b5899e, migration 135; subsequent evidence-only commits need no binary rebuild. Both execution gates and global fleet generation are disabled.
- Next: durable bulk-owned policy drift plus expiry/quota cleanup integration before fleet enablement. Do not replay earlier scale stages or start a new 10000 run as a bootstrap step. This test does not prove simultaneous traffic capacity.

## Durable owned lifecycle — accepted and continuously enabled (2026-10-03)

This checkpoint supersedes earlier statements that fleet generation must remain disabled. Read `docs/BULK_LIFECYCLE_ACCEPTANCE.json` and fresh production gates/scopes before any action.

- Runtime API/worker/canary: clean detached build b789a47639f5299e84093024262d799edaf644fb, migrations 136/137. Subsequent documentation commits do not require rebuilding identical product source.
- Durable policy updates map quota to totalGB, device limit to limitHwid, and lifetime to immutable ownership creation time plus configured seconds. Unknown v3 fields survive full-replacement UPDATE. Manual/non-owned clients are excluded.
- Fresh inactive/expiry/quota evidence creates owned-only BULK_DELETE plans. Refill waits for confirmed deletion and a new fresh deficit; durable rate allowance, immutable identities, PLANNED-before-POST ownership, global identity checks and read-before-retry recovery remain enforced.
- Full three-client cycles passed on panels f06917d6-8127-402b-9c3b-10e44d8e9039 and e1b309e2-6c67-45e0-925b-59f68293253d. Each: create, Output, policy change to 20 MiB / 75 seconds / HWID 3, actual expiry, delete three, three distinct replacements, Output, and owned-only cleanup restoring the exact baseline. All jobs succeeded at attempt=1. The second full run additionally asserted visible_until at least ten seconds before expiry.
- The first observer run crossed concurrent UPDATE reads and stopped. Fresh read and durable cleanup restored its baseline; original FAILED log/marker remain. Its cleanup-only SUCCEEDED record does not count as a full-cycle pass. e71b172 retries only inconsistent read observations during the policy wait, never a mutation.
- Fleet ramp 1 -> 2 -> 6 panels, delete chunks 10 -> 25 -> 50, removed 441 previously inactive owned clients in 15 first-attempt jobs. Nine manual-client hashes stayed identical. This deliberately changes the old 50-record baseline to one live manual client per panel; do not confuse that authorized cleanup with drift in the historical 10000 test.
- Three newly ready panels passed automatic v3 admission after normal Reality stability checks. At acceptance: 12 enabled scopes, each active_users=1, no scope errors, 12 fresh visible Output items, no unresolved lifecycle jobs or pending ownership. API/worker and all 12 target x-ui services active. Output remains API-derived, with aggregate refresh.
- Current global policy remains target=1, quota=0, lifetime=10800 seconds, device limit=0, rate=1/sec, ports=[443]; it is now enabled. No arbitrary higher capacity was selected.
- Continuous authorization: main gate enabled, kill_switch=false, concurrency=1; old bulk canary gate disabled, kill_switch=true, remaining_batches=0. Lifecycle control enabled, auto_enroll=true, cap=64 scopes, initial budget=10000 claims/scope, max chunk=100. Scope expiry is bounded by droplet expiry minus ten seconds and a maximum admission horizon. Existing closed/exhausted scopes never automatically rearm. An executor error closes the main gate; admission cannot reopen it.
- Panel 249a0885-6310-4103-81f4-0ab06071ea1f is intentionally unadmitted due to two historical FAILED CREATE rows. Do not clear or blindly retry them. Expired/disabled/unready panels are not rollout candidates.
- Structural inbound repair now freshly preserves all existing raw client records, unknown client fields and integer precision instead of posting the builder's bootstrap-only list.
- Focused real-PostgreSQL/race tests, full Go tests and clean detached tests passed. Fault tests cover committed UPDATE response loss, partial DELETE response loss, missing-only recovery, policy supersession, admission/gate races and no premature replacement. Actual production quota consumption was not generated: the quota-exhaustion trigger has real-PostgreSQL/HTTP-fault coverage, while production quota fields and actual expiry/replacement were verified.
- Evidence, original failures, staged hashes, health checks and deployment rollback binaries: /root/backups/dob-lifecycle-20261003. /tmp remains unsuitable for builds; use disk-backed GOTMPDIR.
- To stop execution, close the main and old bulk gates and disable lifecycle auto_enroll/global creation. Retain scopes, ownership and jobs for fresh-read reconciliation. Never run a second executor, reset audit rows, or replay completed canaries as recovery.
- Next operational work: observe finite-budget/expiry boundaries and independently reconcile the excluded historical panel if requested. The four requested lifecycle features are accepted for eligible v3 scopes; do not restart the earlier 10000 configured-client experiment.

## Latest Ads UDP / DNS continuation (2026-10-05)

Read docs/ADS_UDP_DNS_HANDOFF_20261005.md and docs/ADS_UDP_DNS_ACCEPTANCE_20261005.json before routing work. API/Worker deployed from clean source e5ec5e8beb5dd533885b252d2757ec1dba02a356. Latest user re-enabled UDP. Managed cached IPv4 DNS is server-direct and working over real Reality; Ads TCP/UDP remain residential and fail closed. Current upstream returns AUTH_REJECTED; slow panel API and cold tunnel latency remain separately unresolved. No AdMob display success or maximum-speed claim.

## Google Ads only continuation (2026-10-05)

Read docs/GOOGLE_ADS_ONLY_HANDOFF_20261005.md and docs/GOOGLE_ADS_ONLY_ACCEPTANCE_20261005.json. User explicitly narrows residential to exactly `geosite:google@ads` plus temporary `domain:browserleaks.com`. Product runtime 14fc055e2df887551f3e311991db9467d92363f6. Saved list verified 38/38; full runtime matrix 37/38, with one pending fresh API read. Preserve existing UDP and direct IPv4 DNS. Earlier upstream AUTH_REJECTED was not repaired or retested.


## Latest kernel OOM recovery (2026-10-05)

Read docs/KERNEL_MEMORY_GUARD_HANDOFF_20261005.md and docs/KERNEL_MEMORY_GUARD_ACCEPTANCE_20261005.json first for the current frontier. This supersedes the one pending runtime panel above without rewriting its original failure evidence. Runtime API/Worker a282d17a82e6de268901c0e041abcef7ddd989bc includes a guarded, journaled KHO/CMA mitigation in bootstrap and activation readiness. All 21 susceptible existing servers were individually rebooted and verified; fresh 38/38 current panels pass API/Xray health and all 34 routing probes. Preserve google@ads plus temporary BrowserLeaks, TCP/UDP and direct IPv4 DNS. Do not replay completed maintenance. Earlier upstream AUTH_REJECTED and actual app ad-display/performance remain separate, untested limitations.


## Latest residential pool continuation (2026-10-05)

Read `docs/RESIDENTIAL_POOL_HANDOFF_20261005.md` and `docs/RESIDENTIAL_POOL_ACCEPTANCE_20261005.json`. Clean API/Worker/static source 7d41d441ff6be4077bdd72f30de65b1739714abe, migration 150. Exact-name atomic/idempotent bulk SOCKS import, selected/all copy/delete, and per-server native 80/20 healthy latency pools are deployed. Full/race/PostgreSQL/browser/real TCP+UDP fault tests passed. Strict canary caught Sanaei API-rule ordering and rolled back; fixed rollout passed 1→6→38 and all 38 original running matrices. Subsequent external live import/edit changed the pool to 11 ads endpoints at revision 594; latest full proof passes 32/38. Six remain unconfirmed, including direct-gRPC deadlines and one diagnostic download timeout; do not claim current fleet verification complete or HTTP-only failure. No agent-created live endpoint fixtures or endpoint deletions. Exact scope, DNS/UDP, profiles/gates and kernel guard are preserved. Before pre-pool binary rollback, reconcile this version with DOB_RESIDENTIAL_POOL_PANELS=none to remove managed pool references. Do not replay old migrations, imports, canaries or reboots.


## Reversible performance continuation (2026-10-05)

Read `docs/RESIDENTIAL_PERFORMANCE_HANDOFF_20261005.md` and `docs/RESIDENTIAL_PERFORMANCE_ACCEPTANCE_20261005.json`. Clean API/Worker/static source 9e98182911beeb4c2b8b4598b350e4c8cc8e9130, migration 151. Residential now has bounded native tuning, preview, timed canary, verified promotion/keep, explicit and durable automatic rollback. Full/race/PostgreSQL/installed-core/browser gates passed. One live canary passed 38 route probes tuned and 38 restored. Final experiment ROLLED_BACK; old profile active, all 12 predeployment endpoints and current profiles/gates preserved. Current controller metadata 24/24 is a different lifecycle population and does not erase previous independent-fleet timeout evidence. Destination targetStrategy ForceIPv4 was removed after real installed-core testing showed it ineffective. Roll back active profiles and verify them with this version before any pre-151 binary downgrade.


## Permanent fleet publication (2026-10-05)

Read `docs/RESIDENTIAL_FLEET_HANDOFF_20261005.md` and `docs/RESIDENTIAL_FLEET_ACCEPTANCE_20261005.json`. Clean runtime source 54cbc731cf1c81b75afcad035b4d0707f1b62d62, migration 152. This supersedes timed/single-server UI restrictions: Select all/Clear selection, default Permanent/no deadline, and all current/future inheritance are deployed. Future admission and rollback share the durable transaction lock. Pending/offline/gated targets are not marked verified. Legacy timed behavior, full/race/PostgreSQL/native-core/browser acceptance passed; production permanent selected canary passed 38 route proofs applied and 38 restored. Global publication was not activated; user can review and Publish permanently. No active profile remains after acceptance. Existing profiles/gates/12 endpoints are unchanged. Do not downgrade until permanent profiles are rolled back and verified.


## Latest UpCloud continuation (2026-10-05)

Read docs/UPCLOUD_HANDOFF_20261005.md and docs/UPCLOUD_ACCEPTANCE_20261005.json. Clean API/worker/static source cb131196167337612de85fc44d3005ffa9eae7ee, migration 153. UpCloud appears alongside DO/Vultr with bearer token, shared proxy/scheduler/SSH/Sanaei/Reality/residential/expiry flow, plan-aware resource budget, complete inventory and durable owned-disk cleanup. Full/race/PostgreSQL/browser and production readback pass; no live UpCloud account exists and real cloud lifecycle remains untested. Two acceptance-only failures exercised automatic rollback; final rollout passed. User's permanent fleet profile 38f9c98e-0acb-4f95-8639-8e65087edce0 is now KEPT/fleet/no deadline with fast_count 13 and TCP Fast Open true, superseding earlier no-active-profile text. Existing configuration preserved. Clean UpCloud accounts and pending storage manifests with this driver before downgrade. Do not replay historical canaries or claim new ad-load/fleet route proof.


## Latest UpCloud preview diagnostics (2026-10-05 UTC)

Read docs/UPCLOUD_PREVIEW_HANDOFF_20261005.md and docs/UPCLOUD_PREVIEW_ACCEPTANCE_20261005.json. API/worker 88f8b50f5d0d3107069c2e495d6b5cc71cb2416b; static/migration unchanged. Misleading generic transport message replaced by safe staged diagnostics for UpCloud, preserving retry/create ambiguity. Tests and selected-proxy synthetic invalid-token HTTP 401 with UI/journal correlation pass. Original screenshot's root cause remains unconfirmed because old preview had no stage logging and unsaved token was not retained. Proxy health gate briefly became unhealthy then healthy. User should refresh and retry validation; inspect reference without reading/logging token. Existing permanent residential publication and settings preserved.

## UpCloud nullable quota continuation (2026-10-05)
- Read docs/UPCLOUD_NULL_QUOTAS_HANDOFF_20261005.md and docs/UPCLOUD_NULL_QUOTAS_ACCEPTANCE_20261005.json. Runtime 5aa7fef66b5ff3e644663b8468d4a724585aa6dd; static/migration unchanged.
- User trace identified account HTTP200 INVALID_NUMBER. Two documented nullable Dev quota fields reproduced the incompatibility; parser corrected with selected-plan fail-closed protection. Tests/deployment/config readback pass. Authenticated user retry remains required.

## UpCloud quota diagnostics continuation (2026-10-05)
- Read docs/UPCLOUD_QUOTA_DIAGNOSTICS_HANDOFF_20261005.md and docs/UPCLOUD_QUOTA_DIAGNOSTICS_ACCEPTANCE_20261005.json. Runtime 4ada688681d03283dc0712c2fb5cc6798f91b3c0; static/migration unchanged.
- Prior nullable-Dev fix did not resolve authenticated user preview. Added safe per-field/type diagnostics with all issues in correlated log. Tests/deploy pass; actual root fix awaits one authenticated retry. Do not claim the null-Dev hypothesis was confirmed.

## UpCloud extension null fix continuation (2026-10-05)
- Read docs/UPCLOUD_EXTENSION_QUOTA_HANDOFF_20261005.md and docs/UPCLOUD_EXTENSION_QUOTA_ACCEPTANCE_20261005.json. Runtime 1ae912189edcac45437b1ede21f17743cd3f6942; static/migration unchanged.
- Live trace confirms one additional null quota rejected at account. Preserve nil extension quotas/usage, retaining strict required-budget checks. Regression/tests/deployment pass; full authenticated preview requires user retry.

## UpCloud server-list query continuation (2026-10-05)
- Read docs/UPCLOUD_SERVER_QUERY_HANDOFF_20261005.md and docs/UPCLOUD_SERVER_QUERY_ACCEPTANCE_20261005.json. Runtime b44d3716e104bbc916caf311d29189940151c36f; static/migration unchanged.
- Account passes; next observed failure is capacity/list_servers HTTP400. Corrected inverted sort_by/order_by per current OpenAPI; contract fixtures and rollout pass. Full authenticated preview remains pending user retry.

## UpCloud regional prices and image labels (2026-10-05)
- Read docs/UPCLOUD_CATALOG_DISPLAY_HANDOFF_20261005.md and docs/UPCLOUD_CATALOG_DISPLAY_ACCEPTANCE_20261005.json. Runtime and changedstatic 46e928d234b4e0c3ee0698a2823863efe3c57103; migration153 unchanged.
- User screenshot confirms authenticated preview success. Added optional region/currency pricing and full distinct image labels; tests/rollout pass; live newprice response pending.


## Latest UpCloud capacity continuation (2026-10-05)

Read docs/UPCLOUD_CAPACITY_STATUS_HANDOFF_20261005.md and docs/UPCLOUD_CAPACITY_STATUS_ACCEPTANCE_20261005.json. Runtime d384c8b054f57c4b24cbea57bc58e05c5317fd04, migration154. Live Upcloud 11 has per-plan resource budgets 2/2/2, provider servers0 and desired5; latest actual create rejected HTTP403/TRIAL_FIREWALL. Durable block survives a successful explicit provider refresh; list/detail show buildable0 consistently. Readable account status ACTIVE is not create permission. Latest read did not expose a positive trial_mode flag; known create rejection remains authoritative until explicit operator recovery. Full paid UpCloud lifecycle remains unverified. Successful live Refresh and list/detail parity verified; full/race/PostgreSQL/fault/migration/browser gates pass. Existing profiles/gates/account settings and permanent residential publication preserved. Before older-worker rollback pause blocked UpCloud accounts; never drop their durable denial evidence to bypass the guard.

## Account deletion continuation (2026-10-06)
- Read docs/ACCOUNT_DELETION_20261006.md and docs/ACCOUNT_DELETION_ACCEPTANCE_20261006.json. Runtime API/worker/static source 43e8c661b6c22f41254c443abbc16c2cee6679d1, migration157 unchanged. Shared durable deletion/UI reliability fixes passed full Go and focused race/PostgreSQL/mobile/desktop acceptance. Upcloud11 was already purged before the patch; live pending jobs0. Backup/rollback under /root/backups/dob-account-deletion-20261006.
- Current protection control was already enabled/fleet/revision9 by this rollout and is preserved; earlier OFF evidence is historical. No live cloud resources were created/deleted for acceptance; no Xray/config/identity changes.


## Optional account rule application (2026-10-06)
- Read docs/ACCOUNT_RULE_APPLICATION_20261006.md and docs/ACCOUNT_RULE_ACCEPTANCE_20261006.json. Clean API/worker/static source bd592fb83b03496d2c26ead6de1b67117efb11e1, additive migration158.
- Accounts > Edit has a saved default-OFF Apply rule changes to existing servers checkbox. OFF preserves existing expiries; ON immutable spec changes queue paced one-at-a-time replacements. Desired-only increase never rotates; ON decrease retires exact excess earliest expiry, OFF waits natural expiry. Timing-only changes do not rotate.
- Deployment reservations capture rules revision. Shared admission/lifecycle locks, unstarted cancellation on OFF, stale-worker fence, durable provider-confirmed delete/backfill, and readiness-before-next-retirement preserve the hard Desired ceiling. Already-admitted deletion outcomes must reconcile after OFF.
- Full Go and focused race/PostgreSQL/mobile/desktop tests pass; server OpenAI source review PASS after two corrected findings. Runtime/static hash verification and config readback pass. All4 accounts OFF, pending new rollouts0, all37 pre-existing expiry timestamps unchanged at acceptance. No cloud create/delete for feature testing. Do not represent fixture coverage as live provider replacement E2E.
- Backup/rollback: /root/backups/dob-account-rule-20261006. Disable/cancel unstarted intents with this version and settle admitted work before binary downgrade; retain additive schema/evidence. Earlier FinalMask/transport experiments remain removed.
