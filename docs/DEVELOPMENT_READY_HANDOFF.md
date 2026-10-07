# Development Ready Handoff

## Current frontier — phase1/2/3 remediation deployed and re-audited
Runtime dacc3329b5e2969415fa7051e34356e39d549c61, schema163, frontend0c7721d unchanged. All PH123-001..010 and supplementary reviewed defects closed in tested scope. Actual API review resp_029a4be631125ff6006ac640e5fdb487d1a7294bff85abb394 PASS. Read PHASE123_FINAL_REAUDIT_20261007.md/JSON, PHASE123_REMEDIATION_ACCEPTANCE_20261007.json and PHASE123_REMEDIATION_BRAIN_DELTA_20261007.json.
163 focused race tests, full Go, retained PG/HTTP phase1/2/3 gates, sixteen shell-model install/upgrade fixtures and five bootstrap tests PASS. Exact committed worker passed real isolated systemd recovery/peer isolation; production hashes match. 49 samples/481.1s healthy, zero restarts, reset ledgers. Native 10 configs/4 panels valid. 6 new successful config create jobs; no new expiry-cycle proof because profiles already use infinite lifetime.
Separate blockers: five Vultr TOKEN_INVALID (persisted Unauthorized classification), one disabled DO BILLING_BLOCKED. Existing deployment-start policy: Direct1/Residential2, lifetime0. Do not restore historical10/600 settings or treat old422 count as current.
Next: diagnose provider authentication facts, then phase4 fair eligible-account scheduling, provider/account resource isolation and explainable blockers. Phase4 not started. Preserve credentials, gates, proxies, Desired and Build spacing. No re-priming. Canonical checkout intentionally detached; push HEAD to checkpoint/final-e2e-20260929.
Earlier overlapping writers paused; this continuation finished against stable hashes. Evidence: /root/backups/dob-phase123-remediation-20261007. Rollback stops both producers and refuses old code over unresolved recovery checkpoints or unapplied successful workflow results; retain additive schema163 and matching units. Shared infrastructure and corrupt-ledger fail-close remain limitations.

## Historical frontier — phase1/2/3 audit initially found ten defects
Read docs/PHASE123_AUDIT_20261007.md and paired JSON/SOURCE_SNAPSHOT first. Verdict REVISE:3HIGH/7MEDIUM. Production remains ac1d91e, schema162/static0c7721d, currently healthy. This audit makes NO product/runtime/config/gate change. Earlier PASS records are bounded historical coverage, not closure of these defects.

Priorities: orphan checkpoint after account purge can poison unrelated recovery; pre-admission capacity waves can falsely expire root supervision; fresh install/standard upgrade do not consistently install split topology. Then checked persistence/UTF8/residential pacing, terminal state writes, async scanner supervision and cross-window fairness. Eight isolated characterization probes reproduce seven findings; packaging selection/source confirms two; one terminal-write finding is static-only. Retained PG/HTTP/race/full-Go and isolated systemd acceptance PASS. Actual server API reviews completed and false positives were triaged.

At10:47UTC all three original production PIDs had NRestarts0, five readiness checks passed, no current-process fault signatures, no deleted-account orphan checkpoint. Native422 configs/47panels checked valid;928 postdeploy-born clients reached >=595s before deletion and1216 delete jobs had later same-scope creates. Counts are point-in-time evidence, not indefinite uptime/mobile traffic proof. Full evidence: /root/backups/dob-phase123-audit-20261007.

Next task is bounded fixes with fault regressions; preserve explicit gates, ownership, unknown-outcome readback, provider restrictions and finite budgets. Reproducer: tools/phase123-audit-repro.sh (isolated bulk_test DB only). Canonical checkout remains intentionally detached; push HEAD to checkpoint/final-e2e-20260929.

## Historical frontier — phase3 progress supervision deployed and native-verified
API/control/panels runtimeac1d91e8a89ea575a15e1a1c95a846c2f68fe0ff, schema162, static0c7721d preserved. Read docs/PHASE3_SUPERVISION_20261007.md and paired ACCEPTANCE/SOURCE_SNAPSHOT. Actual OpenAI API final source review PASS; retained PG/HTTP/race/full-Go phase1+2 and new phase3 gates PASS. Development Orchestrator job phase3-supervision-20261007 COMPLETE.

Independent supervision watches loop, admitted task and declared native phase progress. It cancels and bounded-joins a failed role, then exits; systemd restarts that role, including external SIGSTOP detection. No goroutine replay/abandoned admission slot. Independent control/panels OS roles keep24/40 pools and3/10 admission. Persistent per-role restart state survives process death and paces repeated failure. READY requires observed modules; heartbeat cannot mask a hung task.

The exact committed binary passed SIGSTOP/watchdog replacement, unchanged control PID, new healthy panels heartbeat and duplicate role rejection in an empty isolated PostgreSQL fixture. Production currently has notify units with45-second watchdog and90-second stop deadline. At2026-10-07T10:15:14.285478+00:00, runtime hashes matched, five readiness checks passed, roles retained original PIDs with no automatic restarts during34 samples over5minutes, and healthy restart ledgers reset. Fresh native inventory verified290 configs across37 panels. No current-process supervision/ownership/checkpoint/global-gate fault signature appeared.

First rollout hit the stopped-unit WatchdogUSec=infinity assertion and automatically restored50f9ffb; isolated reproduction confirmed this is runtime state, so the assertion now runs after startup. Second attempt deployed the SAME reviewed artifact. Evidence and backups: /root/backups/dob-phase3-supervision-20261007. Roll back both producers together to checkpoint-aware50f9ffb with their MATCHING original simple units; retain schema162 and restart ledgers. Never run the old non-notifying binary with notify units or checkpoint-unaware code with unresolved checkpoints.

No account/profile/gate/proxy/routing/protection reset or new Resume. Operator/provider/ownership/quarantine/finite-budget/unknown-outcome fences remain authoritative. Common host, DB and network remain shared; injected-clock long-phase coverage and accelerated fixture watchdog are recorded limitations. Next bounded work: queue fairness, per-account/provider resource isolation and recovery decision diagnostics. Canonical checkout stays detached intentionally; push HEAD to checkpoint/final-e2e-20260929.


## Historical frontier — durable recovery ledger correction
Runtime/API/worker50f9ffb6ac20012c0e57e068fbde2aef0905833b, schema162; static0c7721d unchanged. Read docs/RECOVERY_LEDGER_FIX_20261007.md and paired ACCEPTANCE/SOURCE_SNAPSHOT. PH12-001/002 are CLOSED; equivalent lifecycle completion, failed-active-key health race and unbounded deferred recovery bookkeeping are also corrected. Actual API review PASS; final PostgreSQL/race/full-Go/phase1+2/rollback-launcher tests PASS; exact built worker systemd isolation/restart fixture PASS. Development Orchestrator job recovery-ledger-fix-20261007 COMPLETE.

At2026-10-07T09:17:13.931255+00:00, API/control/panels were active, actual hashes matched, no automatic restart or new current-process error signature, five readiness checks PASS.304 output configs matched fresh native inventory across33panels;35Direct/280Residential on later share sample;20 new create and19 new delete jobs succeeded by09:16:42Z. Existing client expiry/replacement continued. UpCloud113 had2READY servers/Desired5. Bounded observation, no mobile traffic or indefinite-uptime guarantee.

The first rollout triggered a real guarded rollback because acceptance observed prior-process heartbeat before new PIDs published. The deployment wait now requires BOTH current-PID heartbeats; the second attempt deployed the SAME approved binaries. No product change occurred between attempts. Config snapshots preserved gates/profiles/account/routing/proxy/protection settings. No new Resume/recharge required. Evidence and before-binaries: /root/backups/dob-recovery-ledger-fix-20261007.

Rollback MUST use tools/recovery-ledger-rollback.py (stop BOTH producers, verify zero durable checkpoints, then restore); schema162 stays additive. Never start old checkpoint-unaware code over unresolved rows. Future work remains living-but-stuck module supervision, wider fairness and independent health evidence; common host/DB/network remain shared. Do not bypass provider/ownership/unknown-outcome/quarantine/policy/budget gates. Canonical checkout remains detached intentionally; push HEAD to checkpoint/final-e2e-20260929.

## Historical frontier — phase1/phase2 re-audit found two HIGH defects
Read docs/PHASE12_REAUDIT_20261007.md and JSON before any new implementation. Runtime951ea1a remains active and source/runtime hashes match. Existing acceptance and exact deployed-binary isolated systemd restart fixture PASS, but new real-PG fault probes reproduced PH12-001 (failure-ledger read errors can reach deployment bypass and fake healthy scan) and PH12-002 (failure/backoff writes are best-effort, allowing repeated recovery without durable delay). Both are UNFIXED, inherited legacy recovery caller defects. Current audit verdict REVISE; earlier PASS is historical coverage, not closure of these findings.

No production restart/product edit/gate/policy change during this audit. All five readiness checks were green through08:39UTC; native398 published configs/45 panels checked valid,212 post-phase2-born clients completed600-second expiry/deletion, current worker pool waits0. Readiness does not detect the two selective ledger faults. UpCloud newest replacement was PROVISIONING at08:37UTC, not yet READY-confirmed.

Next concrete work: checked read admission before any backoff bypass; checked asynchronous failure/clear persistence with bounded per-item fencing and truthful progress. Preserve unknown-outcome, scope/ownership/provider guards and explicit operator policy. Repro source/evidence at /root/backups/dob-phase12-reaudit-20261007. Orchestrator audit completion means report/test workflow complete, not product approval.

## Prior frontier — phase2 deployment evidence

Runtime/source checkpoint: 951ea1a5852e716d09f4362e75f1a4ba51ca3c7a. API plus control and panels worker services are active. Static0c7721d/schema161 unchanged. All five readiness checks passed through 2026-10-07T08:18:50.474410362Z; source and running binary hashes are verified. Read docs/WORKER_ISOLATION_20261007.md and its ACCEPTANCE/SOURCE_SNAPSHOT.

The user's credit recharge resolved the API blocker. Genuine OpenAI review found two issues (startup ownership-monitor gap and DB admission reserve); both are corrected, final review PASS, real PG/race/HTTP/full-Go/systemd isolation tests PASS. Development Orchestrator worker-isolation-20261007 is COMPLETE. No recharge or additional Resume is pending; the actual client gate remains enabled from06:23:13Z.

Production after rollout:37 create and36 delete jobs succeeded by08:18:24Z; same-scope replacement continued. Public HTTP200 shares at08:18:29Z had Direct53/Residential379 lines. Six production-only samples had zero pool waits, advancing control scans and mutation successes96 to136. Both UpCloud servers were READY at observation; Desired5 unchanged. Samples are bounded evidence, not indefinite uptime/mobile traffic proof.

All account/rule/profile/gate/routing/proxy/protection settings were preserved; only worker service topology and binaries intentionally changed. Previous caf3f0d binaries/topology are the rollback baseline in /root/backups/dob-worker-isolation-20261007. Stop BOTH split workers before any old all-role startup. Do not clear quarantine, rearm budgets or fabricate auth. Canonical checkout remains detached intentionally because the named branch is held in an older archive; push HEAD normally to checkpoint/final-e2e-20260929 without resetting that archive.

Next scope: living-but-stuck module progress supervision, queue fairness and independent health evidence. Roles still share host/database/network; no blanket self-repair guarantee. Continue from source snapshots and live facts without re-priming.

## Historical frontiers
The following entries preserve earlier observations and are superseded by the current frontier above. Old Resume/recharge requests and earlier shared-worker limitations are historical, not current actions.

## Modular recovery frontier — 2026-10-07
Read docs/MODULAR_RECOVERY_20261007.md and paired ACCEPTANCE/SOURCE_SNAPSHOT JSON. Clean API/worker revision caf3f0de76d142b7e8ded2923a429b00fd4ba50c is deployed; static0c7721d/schema161 unchanged. Native HTTP fault, PostgreSQL/race/full Go and final server OpenAI review PASS. Config hashes and actual running binary hashes verified; all4 readiness checks pass.

This supersedes earlier live-enabled observations: at05:28:43Z a lifecycle BULK_DELETE on a retiring panel returned ErrExecutionGated, globally closing the gate. Job eb7010c2-94ab-4158-8eb2-4fd3d0ef2bb1 subsequently OBSOLETE. New code durably defers typed policy refusals without global closure, refunding budgets, clearing quarantine or blind POST. All queued origins have typed panel isolation; native wait/circuit provenance, complete identity validation, exact-target/ownership/expiry/provider permission guards, atomic event/backoff writes and below-Desired/full-provider-ceiling retirement are covered. Lifecycle has6/account1 bounded lanes; client scan/job and stalled-lane telemetry distinguish policy GATED from success.

At 2026-10-07T06:17:18.545926+00:00, main gate remains exactly the existing05:28 administrative closure; authenticated Configs Resume is REQUIRED. Residential share remains0. No admin browser session is available to this agent; do not raw-update the gate, fabricate tokens or silently broaden authorization. Finish with real native generation/share/expiry-cycle proof after the user resumes. Later native proof at06:19Z supersedes the earlier timeout: Upcloud113 DELETE861c4a56 succeeded at06:17:25Z after automatic reconciliation (attempt3), scheduler backfill began and CREATEe93656db is verifying at attempt1. The second old server remains EXPIRING; Desired5 is unchanged. Complete READY replacement and its Output are not yet verified. No new global executor failure/panic/ambiguous retry SQL after deployment, but the client gate was closed, so this is not production mutation-coverage proof.

Remaining architecture scope: shared worker process/memory/DB pool and global client executor lock; no forced goroutine abandonment; common proxy-health evidence and wider fairness/process supervision remain future work. Provider locks/billing/trial restrictions and expired/exhausted budgets still require their legitimate resolution. Evidence, original failures, clean freeze and rollback: /root/backups/dob-modular-recovery-20261007.

## Historical frontier — UI recovery and verified client replacement, 2026-10-07
Static source `0c7721d4521beed9ba0b7a4f6fba12114b580cc1` is deployed; runtime API/worker remains00a4b185929185224c38719bd8c1ec9a71dc35c6, schema161. The user clicked authenticated Resume at04:59:18Z. Main gate stayed enabled; native client creation,600-second expiry deletion/replacement and actual class shares are verified. Latest observation05:16:32Z: Direct56/Residential443,996 succeeded bulk creates,344 succeeded bulk deletes,53 panel/inbound scopes with same-scope replacement. Counts vary with fleet lifecycle. No Resume action remains pending.

Configs null-innerHTML refresh race is fixed. View-scoped GET timeout/abort/identity fences, single-flight polling, retained last-good status,5..30second retry, online/visibility wake and account callback guards prevent the tested stale/hung response cases. Empty snapshot counts are unavailable, not fabricated zero. Browser390x844 fault tests, PostgreSQL/API race, full Go and final OpenAI review PASS. Static deployment preserves settings and running binaries; actual configured admin HTML/static hashes verified. Read `docs/UI_REFRESH_RECOVERY_20261007.md`, its SOURCE_SNAPSHOT and ACCEPTANCE JSON. A tab already open only needs one reload for new JavaScript.

Future work must use actual evidence; do not promise indefinite uptime, automatically clear quarantine, reset finite budgets or fabricate admin tokens. Separate provider/fleet issues remain: UpCloud trial endpoint restrictions/expiry capacity deadlock, app recovery ambiguous next_retry_at, earlier x-ui expiry restarts and Vultr232 credit classification. Evidence/rollback: `/root/backups/dob-ui-refresh-recovery-20261007`.

## Next development scope — isolation risk forecast, 2026-10-07
Read `docs/ISOLATION_RISK_FORECAST_20261007.json`. This is a source-reviewed forecast, not deployed remediation. Main current exposures: known remote errors from non-lifecycle/manual client jobs can still close the global gate; global mutation serialization spans remote I/O; worker subsystems share process/DB resources; readiness does not directly measure client-mutation or Output progress; proxy health has a correlated default endpoint; account lifecycle can wait below Desired before detecting a full provider ceiling; provisioning retry query is schema-confirmed ambiguous and its error is ignored. Preserve unknown-outcome reconciliation, ownership, finite budgets, quarantine and operator policy. First implementation scope should classify all mutation origins with scoped remote failure handling and explicit progress supervision; do not just remove the executor lock or increase concurrency. Current live build gate is enabled, no active budget is exhausted, and two old PENDING jobs have expired scopes. No production changes in this audit. Follow with bounded task/process isolation, independent health evidence, capacity deadlock/retry query fixes, and fault matrix acceptance.

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


## Planned delete recovery (2026-10-06)
- Runtime e29dd72eb9828bea1d700324511c9534edd7a23a, schema158, current status UI correction retained. Read docs/DELETE_RESUME_20261006.md, docs/DELETE_RESUME_SOURCE_SNAPSHOT_20261006.json and docs/DELETE_RESUME_ACCEPTANCE_20261006.json.
- Root cause was an existing planned/attempt0 DELETE returned forever as a no-op; the oldest retirement blocked later account work. Resume planned after exact identity checks and optimistic running claim, preserving existing unknown retry and all network/provider gates. Full Go, fault, race and isolated PostgreSQL 12-concurrent-claim tests pass; source review PASS.
- Both pre-existing Sara vul 2 target retirements now DELETED with authoritative provider absence and one DELETE attempt each. Backfill has started; latest acceptance records provisioning progress, not blanket fleet readiness. Backup and rollback /root/backups/dob-delete-resume-20261006. No migration or configuration changes. Do not replay the two confirmed deletes.
- Earlier all4 OFF snapshot is historical: user enabled all four controls; ON alone queued no rule replacement, Lifetime remains59–62min. Current status UI shows ON / No replacement queued and uses fresh Save readback (8569ab2, unchanged asset included in runtime release).
- Latest readback: both replacements now have provider IDs (deployment284fb33c-3961-4cfe-91cd-33e5dc93c3c1 ande335a2f0-9f2a-45b4-84e3-f460c4efd818). First reached INSTALL_FAILED at14:09:35Z with installer reboot remediation exhausted. Final Ubuntu26.04 readiness had working SSH/package/DNS/HTTPS but reboot_required=yes. Earlier SSH timeouts were transient. Second build was still progressing. Both delete operation attempts remain1. Investigate installer reboot-required separately without replaying cleanup.


## Installer bootstrap recovery (2026-10-06)
- Runtime 7a19f0b13814840a7fcf7ef81b75806bf2fac7cf, schema158 unchanged. Read docs/INSTALLER_BOOTSTRAP_20261006.md, docs/INSTALLER_BOOTSTRAP_SOURCE_SNAPSHOT_20261006.json and docs/INSTALLER_BOOTSTRAP_ACCEPTANCE_20261006.json. Full Go, race/fault/isolated PostgreSQL and source-review PASS; clean runtime hashes and unchanged operator controls verified.
- Removed unconditional recovery promotion before bootstrap completion. Activation uses deployment lease and durable prerequisite evidence; premature waiting repairs to first unfinished step with attempts/backoff intact. Reboot intent is boot-ID fenced; one pre-guard accidental reboot correction requires later successful guard completion. Do not clear readiness markers or reset attempt journals.
- Live: 46.101.148.252 (Sor oc) and 192.248.172.175 (Sara vul 2) automatically recovered from premature waiting; 45.63.97.160 completed interrupted provisioning. All three PANEL_COMPLETE/READY with x-ui active and TCP443/2053 listening; KHO disabled, boot changed, CmaTotal=CmaFree=0, reboot-required=no. Further natural builds are recorded separately. No synthetic provider resources.
- Backup/atomic rollback /root/backups/dob-installer-bootstrap-20261006. Source pushed to canonical branch. Old global verified=false reflects historical broader fleet scope, not an unresolved installer fault. Preserve all four user ON controls and existing Desired/Lifetime/residential/guardian settings.


## Readiness SSH loss follow-up (2026-10-06)
- Runtime 5628300b76e38e1fc6088028d881abab95a690e9, schema158 unchanged. Read docs/INSTALLER_READINESS_20261006.md, docs/INSTALLER_READINESS_SOURCE_SNAPSHOT_20261006.json, docs/INSTALLER_READINESS_ACCEPTANCE_20261006.json. Full Go, focused race/fault/isolated PostgreSQL and final source review PASS. Clean running binary hashes, API/worker health and unchanged controls verified. Backup/rollback /root/backups/dob-installer-readiness-20261006.
- During initial live verification, S1 builds5ad55938 and4a8e4ea2 were falsely finalized on SSH_DISCONNECTED immediately after scheduled reboot under7a19f0b. Both were already deleted by normal lifecycle; do not revive or replay. Replacement5b8df545 (24.144.121.130) now PANEL_COMPLETE/READY.
- Readiness-specific SSH_DISCONNECTED and plain/wrapped EOF now retry within existing budgets; collection stops immediately before false semantic diagnoses or automatic remediation, and host-key failures remain permanent. Global mutation outcome classification is unchanged.
- Final readback 2026-10-06T15:06:48.965661+00:00: S1 3/3, Sara vul17/17, Sara vul2 15/15, Sor oc3/3 READY; zero provisioning/retiring. Three earlier repaired/sample servers retain kernel and service proof. No new live EOF interruption deliberately caused: that fault path is fixture-tested. All user account/routing/residential/guardian settings unchanged.


## Fleet hardening continuation (2026-10-06)
- Read docs/FLEET_HARDENING_20261006.md, paired SOURCE_SNAPSHOT and ACCEPTANCE JSON. This source checkpoint does not itself mean deployed; acceptance records actual rollout state. Migration159 is additive.
- Preserve operator controls including the newly added Vultr232 account, Desired/Lifetime/apply-existing switches, all routing/publication/profiles and execution gates. Do not restore historical all4 snapshots.
- Bounded and fair recovery, initial SSH budget/boot circuits, durable admission snapshots, sticky guardian receipts, Xray process/listener proof, per-process FD pressure, truthful dashboard/history, queue draining, observation retention and worker-progress readiness. External client/HA/TLS/surge dependencies remain explicit.
- Temporary guardian binary rollout scope uses DOB_GUARDIAN_UPGRADE_PANELS; empty=all, none=no binary changes. Existing policy reconciliation remains active. Rollback evidence /root/backups/dob-hardening-20261006.

- Final deployment: clean runtime 8e75d91c0f12573229abbbf51362995a45203f5a, schema159. Go/race/isolated PostgreSQL/real nft fault tests and final source review PASS.1→6→all guardian rollout:42 reachable binary proofs,6 unchanged x-ui PIDs, new future-node process/listener proof; an already-DELETING SSH-unavailable node remains outside binary proof. Natural post-release builds completed.
- Read acceptance before making uptime claims: first migration-permission failure caused a brief management outage and automatic rollback; second runuser PATH preflight also rolled back. Third attempt passed and script permissions/PATH were fixed. Server x-ui was not restarted by API/worker deployment;6-node proof verifies unchanged PIDs.
- Existing settings preserved including all6 current accounts. S1/Sor oc warning and Upcloud113 TRIAL_FIREWALL remain provider restrictions. Dynamic resource pressure is visible as degraded. Old2 SSH failures retired before new deployment, not proof of the new budget. Final guardian upgrade allowlist removed; future managed nodes inherit current policy. Source revision and deployment-script-only follow-up are deliberately distinguished.

## 2026-10-06 stable routing follow-up
Source tests/review pass for residential turnover fingerprint and independent membership receipts. Live canary pending; see ROUTING_FINGERPRINT_20261006.md. Preserve operator settings; use scoped deployment selector and worker-only rollback.

## Delete completion continuation — 2026-10-06
Read docs/DELETE_COMPLETION_20261006.md and paired SOURCE_SNAPSHOT/ACCEPTANCE JSON. Source fixes atomic operation/lifecycle completion and exact historical succeeded-receipt replay. Initial incident: Vultr232 first DELETING already succeeded at provider;14 further expired READY blocked behind it. Tests/full Go/race/isolated PostgreSQL and final source review PASS. This source checkpoint alone is not deployment evidence. No user configuration changes. Routing fingerprint b5ca964 had already reached fleet: documentation now records scoped turnover proof and unresolved native x-ui expiration restarts.

- Deployment confirmed: workerbb74ff2424cf7c6c5c5b7fa1052504a1faacf97a, API8e75d91/static/schema159 unchanged. Historical Vultr232 succeeded receipt now local DELETED; attempt1 and operation timestamp unchanged; stranded completion count0. Backfill signal committed and next expired item advanced. Read latest acceptance for subsequent natural build proof.
- Canonical path currently uses detached source checkout; canonical branch remains checked out in an older dirty archive. Do not discard that archive. Push current reviewed HEAD explicitly to refs/heads/checkpoint/final-e2e-20260929, only fast-forward. Existing/new operator accounts and settings must be preserved.

- Final21:06 UTC boundary: local completion repair is verified and stranded completed deletes0. Original `Vultr232` natural create now reveals HTTP400 insufficient additional credit; current code wrongly labels it TRANSPORT_ERROR. First replacement failed, later retry started, no READY replacement proven.14 expired resources remain. Next code task: fix credit/create-error classification without blocking legitimate cleanup or erasing unknown-outcome evidence. Owner funding is required for actual replacement; do not charge automatically. Newly added `Vultr 232` (with space) is separate and already had2 READY. Native x-ui expiry restarts remain a separate open objective.

## Latest UpCloud trial checkpoint — 2026-10-07 (Tehran)
Read docs/UPCLOUD_TRIAL_HANDOFF_20261007.md, docs/UPCLOUD_TRIAL_ACCEPTANCE_20261007.json and docs/UPCLOUD_TRIAL_SOURCE_SNAPSHOT_20261007.json first for this task. Runtime API/worker fc971058a555718cdd178cd226b038c65b79383f, static1384d16, migration160. Upcloud113 opted in; two live READY servers, panel3389, VLESS/Reality443 and34 native routing proofs on the first current panel. Replacement panel route tests await an observed DIRECT class client; no false pass claimed. Provider locked firewall remains on and IPv4-only. All12 residential proxy ports incompatible, so protected traffic remains fail-closed. No paid upgrade. Trial capacity2 full; no remaining TRIAL_FIREWALL block. First affected kernel failed and auto-cleaned; vendor GRUB order fix is deployed/tested for future builds, no live affected-kernel repair claimed. Preserve regular lifetimes/rotation and all other settings. Full phone/client traffic remains unverified; do not claim unrestricted/forever-free service or residential success. Prior native x-ui restart and Vultr232 billing classification work remain open.

## Latest diagnosis — UpCloud empty Output, 2026-10-07 07:3x Tehran
Read docs/UPCLOUD_OUTPUT_DIAGNOSIS_20261007.md before further changes. Both prior UpCloud servers are now expired and remain occupied because shouldWaitForDeficitBackfill(5,2) returns before the full-provider-capacity retirement branch. Serial per-account lifecycle selection then starves the second expired server. Initial deployment acceptance did not prove rotation. Status DIAGNOSED_NOT_FIXED; runtime fc97105 unchanged. Output expiry filter is correct; repair lifecycle capacity ordering without changing Desired5 or exposing blocked residential links. Native API counts and server OpenAI review saved; no production mutations.


## Latest fleet Output diagnosis — 2026-10-07 07:50 Tehran
Read docs/OUTPUT_GLOBAL_DIAGNOSIS_20261007.md first. User video and exact shared HTTP responses show Residential0 / Direct1. Confirmed syslog trigger: at2026-10-06T21:57:16Z a per-panel Sanaei runtime circuit (retry30s) caused the client executor to persistently close the global mutation gate. Gate remains closed;600-second Residential clients expired with no refill; current59 route-eligible preserved clients have no qualifying profile ownership. Both services active. Earlier UpCloud-only diagnosis remains separate and is insufficient for the fleet symptom. The same-time ambiguous next_retry_at SQL is unrelated to gate closure. Status DIAGNOSED_NOT_FIXED; runtime fc97105 unchanged. Next: reviewed per-panel transient deferral preserving uncertain-outcome/invariant closure, durable reason visibility, controlled authenticated capacity resume and fresh native/classified Output verification. No gates or user settings changed; do not remove ownership filters or blindly reopen/clear failed jobs. Evidence /root/backups/dob-output-global-diagnosis-20261007.
