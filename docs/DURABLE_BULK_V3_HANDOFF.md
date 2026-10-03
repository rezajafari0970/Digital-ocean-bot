# Durable bulk v3 continuation — 2026-10-03

Current accepted frontier: stage 10000, fully cleaned up. Read the final stage-10000 section and BULK_V3_10000_ACCEPTANCE.json; earlier stage notes are historical.

Authoritative baseline before this change: b83e92fe39efb17081db60bb65641da0a82a2efc.
Three unfinished edits and cmd/xui-bulk-route-inspect existed at continuation. Their original patch and utility were preserved in .local/bulk-v3-20261003 before integration. Legacy policy LimitIP behavior was restored after its existing test failed; new v3 batch payloads use LimitHWID.

## Implementation

- Migration 000134 extends client_mutation_jobs with BULK_CREATE, result evidence, immutable batch payload protection, and a unique unresolved batch per panel/inbound. Ownership links to its durable mutation job.
- FastFill only plans. UUID/email/policy payload and PLANNED ownership commit atomically. It reuses the durable token bucket; lost planning allowance is conservative and never refunded into a burst.
- The existing worker executor runs bulk jobs, sharing a pinned-connection PostgreSQL advisory lock with single-client jobs. No second executor loop is introduced.
- The existing execution gate and a new finite bulk gate must both allow execution. Bulk gate has an exact panel/inbound, expiry, max chunk (default 10), and claim budget. Each claim consumes one budget unit. A retry requires explicitly reopening/replenishing the finite gate after reviewing observed state; fail-close errors do not automatically reopen it.
- Before every POST, fresh inbound and global-client reads establish desired state. Only missing identities are posted. Gate/lifecycle row locks fence the bounded POST. After all response outcomes, fresh reconciliation confirms individual ownership; created counts alone never activate ownership.
- V3 global reads verify UUID, email, quota, expiry, HWID and enable; one inbound snapshot per observation confirms runtime membership/flow. Global reads are currently per observed client, so higher chunk performance needs measured acceptance.
- BULK_CREATE executor is v3-only during initial rollout. Unsupported bulk routes fail closed; retained legacy adapter is not activated implicitly by this executor.
- Old ownership recovery excludes batch-linked ownership. Bulk-owned inbounds avoid legacy full-inbound rewrites. Their capacity is observation-only in that path; broad automatic v3 policy updates/expiry cleanup require journal integration before fleet enablement.
- Existing single-client failure returns now propagate terminal errors so the worker closes its gate rather than reporting a failed job as success.

## Canary and recovery

Clean-built cmd/bulk-client-canary requires the main and bulk gates closed, a disabled global creation policy, no pending scope jobs, and exactly one preserved baseline client. It plans ten clients at scoped rate 10/sec with quota 100 MiB, 30 minute lifetime and HWID 2. Worker-only execution, output visibility and durable DELETE cleanup are verified; the baseline hash must be restored.

On partial/ambiguous failure, leave gates closed. Read the printed job ID and use the same clean-built utility with -recover-job <id>. It reads the original immutable plan, fresh-verifies matching clients, closes creation and queues only owned matching identities for DELETE. It refuses conflicting or still-running states. It never creates replacement identities. After process kill, read before any retry. No canary is silently retried.

## Validation and rollout

Focused tests include real isolated PostgreSQL concurrent planning, atomic ownership/journal persistence, gate/claim limits, executor lock contention, partial server commit plus lost response, missing-subset retry, and no POST after observed completion. Run focused race tests and go test ./... before a detached clean build.

Deployment/canary acceptance will be appended only after live evidence. Initial acceptance is 10 clients; 25/50/100/250 and fleet enablement remain separate measured gates.

## Live acceptance completed

Runtime source af1a541799088c6d3c89af675d5f6b02f82cd84c deployed from detached clean build. Migration 000134 applied. Job 59956b9c-543f-4ba9-a344-d2f82b5c664f created/verified ten users in one attempt; all ten appeared in Output. Ten durable DELETE jobs succeeded on their first attempt. The original single-client baseline hash was restored; Output has one baseline item and no canary items. Both gates closed, main concurrency=1, bulk remaining_batches=0. API, worker and target x-ui active. Capacity 1/1, deficit=0, no error. See BULK_V3_ACCEPTANCE.json for exact evidence and limitations.

Next: measured 25-client gated canary, then 50/100/250 only after acceptance. Do not enable fleet generation yet. Durable bulk-owned policy drift and expiry cleanup must be integrated before broad enablement. The canary utility now accepts only 10 or 25 users; larger stages require reviewed bounds and unchanged cleanup guarantees.

## Stage 25 accepted

See BULK_V3_25_ACCEPTANCE.json. Two isolated 25-client runs passed, each one BULK_CREATE attempt and 25 first-attempt DELETE jobs. The second run repaired the resource-measurement evidence after the first SSH output was truncated. Second create verification: 1516 ms; Output observation after create verification: 1003 ms; cleanup: 24573 ms. A complete 56-sample window includes creation and cleanup. Sampled combined x-ui/Xray RSS max 386104 KiB; CPU max 59.57% of one core; MemAvailable minimum 1143000 KiB. Baseline restored, gates closed, capacity 1/1.

Current next gate: 50-client canary. Do not interpret two 25-client runs as a 50-client batch test. Use a disk-backed clean worktree and GOTMPDIR: /tmp is tmpfs and nearly full. Production API/worker remain on af1a541 because this stage changed only canary/inspection utilities and documentation.

## Stage 50 accepted

See BULK_V3_50_ACCEPTANCE.json. Job 20c948e0-db84-449e-83f1-69cd9020b5e8 created and fresh-verified all 50 clients in one attempt (2017 ms). All appeared in Output; all 50 durable DELETE jobs succeeded on their first attempt (50113 ms cleanup). The original single-client hash was restored, generation CLOSED, test Output empty, both gates closed, no pending jobs or new jobs outside the target scope. Fleet ownership is ACTIVE=1813, DELETED=159.

The complete 101-sample resource window covers create and cleanup. Combined x-ui/Xray sampled RSS maximum 283804 KiB; CPU maximum 25.83% of one core; minimum MemAvailable 1329324 KiB. These are one-second samples, not instantaneous peaks. API, worker and target x-ui remain active. Stored capacity is 1/1 but its observed_at predates this test; live restoration is proven by fresh Sanaei readback, not by treating that snapshot as new.

Current next gate: measured 100-client canary. The utility currently allows only 10, 25 or 50. Preserve worker-only execution, finite scope, durable rate budget and journal cleanup. Fleet generation remains disabled pending durable bulk-owned policy drift and expiry/quota cleanup integration. Runtime API/worker remain af1a541; clean-built canary tool is ce9c44d9967554a58a26996f5317f0c989ca5256.

## Stage 100 accepted

See BULK_V3_100_ACCEPTANCE.json. Job ca287033-c7bb-414d-b325-9774eb8e7b96 created and fresh-verified all 100 clients in one attempt (2033 ms). All were observed in Output. All 100 durable DELETE jobs succeeded on the first attempt (99292 ms cleanup). The original one-client hash was restored; generation CLOSED, no test Output, no pending jobs, both gates closed. Fleet ownership ACTIVE=1813, DELETED=259.

The complete 121-sample resource window covers create and cleanup: sampled combined x-ui/Xray RSS max 139252 KiB; CPU max 24.86% of one core; MemAvailable minimum 1521876 KiB. API, worker and target x-ui are active. Stored capacity remains 1/1 with an older observed_at; fresh Sanaei readback proves restoration. Runtime remains af1a541, canary utility f122101e1ab9d8a066bb04054c7e71c5b6d81be1.

Current next step: prepare the measured 250-client stage. Do not only extend the allowlist: current canary passes count as rate to BulkAllowance (rate max 100/sec, bucket capacity one second), waits for a single full allowance, has a four-minute total deadline, and the sampler caps at 120 seconds. Durable admission must respect 100/sec and recovery; cleanup and sampling deadlines must cover serial 250-client deletion. Fleet remains disabled until durable bulk-owned policy drift and expiry/quota cleanup integration.

## Stage 10000 — target verified, cleanup in progress

User requested a direct jump to the 10000-client stage. A longer-lived 2 GiB target was selected: panel f06917d6-8127-402b-9c3b-10e44d8e9039, inbound 1, with 50 preserved clients. Durable run ad494773-1793-41f2-ba79-8909e30c2657 added 9950 test clients in 100 BULK_CREATE jobs, all attempt=1, and fresh-verified 10000 total clients plus 9950 visible test Output entries at 10:52:09 UTC. Creation/verification took 942416 ms. This proves configured capacity, not concurrent traffic sessions.

Migration 135 and durable BULK_DELETE provide owned-only batch cleanup. Installed target bulkDel contract was inspected. Fresh global list reads replace per-client HTTP reads; every relevant identity still receives conflict and inbound-sharing checks. Scope locks, finite gates, durable ownership and worker-only execution remain enforced.

Cleanup batch two committed, but its 15-second readback expired. Worker closed both gates. The first explicit recovery also stopped before mutations when its global-list HTTP read timed out. Clean-tested/deployed revisions 9a58559 and ae5ccdb separate bounded DB diagnostics from verification, allow 45 seconds for post-delete verification, and 30 seconds for the global HTTP read while retaining caller deadlines. Fresh reads reconciled job b10b2935-b607-4082-85b4-9f670c431de0 to SUCCEEDED at attempt=1, with its error audit retained and no duplicate POST.

Cleanup is currently running through scale-recovery2.log in /root/backups/dob-bulk10000-20261003. Do not launch a competing utility or open gates separately. Read PRODUCTION_REVISION_STATUS.json and the live durable run first. The read-only acceptance collector is waiting for cleanup and resource sampling to finish. Original scale-status.txt and scale-recovery-status.txt intentionally remain FAILED. Full acceptance requires the final baseline hash, DELETED=9950, test Output=0, closed gates, healthy services, and acceptance.json. Fleet policy remains disabled.

## Stage 10000 — accepted after cleanup recovery

This section supersedes the preceding in-progress checkpoint. See BULK_V3_10000_ACCEPTANCE.json for machine-checked production evidence and original failure audit.

- Target: panel f06917d6-8127-402b-9c3b-10e44d8e9039 / inbound 1. Existing 50-client baseline preserved, including manual/non-owned identity; 49 baseline clients were already disabled and expired before the run, and final Output correctly contains only the one eligible baseline client; 9950 durable owned test clients added in 100 worker-only BULK_CREATE jobs. All CREATE jobs succeeded at attempt=1. Maximum chunk 100 and admission cap 100/sec; observed target verification time 942416 ms, not a measured 100/sec sustained throughput claim.
- Fresh Sanaei state reached exactly 10000 clients. All 9950 test clients were visible in Output; the recorded observation age at target was 1.125092 seconds. Quota 100 MiB, HWID=2, enabled state, lifetime and Reality flow were checked against the immutable plan.
- All 9950 test clients are now DELETED across 100 BULK_DELETE jobs, all journal attempts=1. The second DELETE had committed but timed out during readback; explicit fresh inbound/global absence completed that original journal row as RECOVERY_VERIFIED without a second POST. Its historical error remains for audit. The first recovery read also timed out before any mutation. Both original FAILED marker files remain unchanged.
- Bounded recovery patches 9a58559 and ae5ccdb passed focused real-PostgreSQL/race tests, full Go tests and clean detached tests/builds. Current production API, worker and canary runtime is ae5ccdbe8d41a77078c12c371297bc6543b5899e, migration 135. API/worker are active with zero automatic restarts; target x-ui is active.
- Final baseline SHA256: d276065bc13226fe33c06368d5bf88cd3ff8e5ce2e64ac114de357c2ff2defd6. Generation CLOSED, scale run SUCCEEDED, no pending scope jobs, test Output=0, existing owned ACTIVE=49 unchanged. No new bulk jobs outside the target scope. Both gates closed, main concurrency=1, bulk claim budget=0, global creation policy disabled.
- Resource evidence comprises 2178 one-second samples over 18 bounded windows. Sampled combined x-ui/Xray RSS maximum 369984 KiB, CPU maximum 100.69% of one core, minimum available memory 930808 KiB. Recorded gaps between sampling runs occurred while recovery gates were closed. These are sampled maxima, not instantaneous peaks.
- Recovery cleanup elapsed 871930 ms; total wall time including investigation, patches and recovery was 3109.706 seconds. Original logs, contracts, test logs, resource windows and evidence hashes are preserved under /root/backups/dob-bulk10000-20261003.

The 10000 stage is configured client capacity and Output acceptance: 9950 new enabled clients plus 50 preserved records, of which 49 were already disabled/expired. It is not evidence of 10000 enabled users or a concurrent traffic benchmark. No repeat of this stage is required merely to continue development. Next product gap: durable policy drift and expiry/quota cleanup for bulk-owned clients. Source entry point: internal/panels/usercapacity/service.go, reconcileRuntimeLocked bulkManaged branch currently calls observeBulkCapacity and continues before legacy policy/expiry handling. Keep fleet creation disabled until those lifecycle paths are journal-integrated and accepted. Any new production mutation still requires fresh live scope/gate/job checks; never reuse this completed run for new identities.
