# Durable bulk v3 continuation — 2026-10-03

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
