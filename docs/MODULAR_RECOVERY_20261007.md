# Modular recovery — 2026-10-07

## Scope and evidence
Baseline a6b1826e44b418e6dab85188a97b23fe8d829b30. This release adds bounded isolation and recovery to the existing native driver architecture. It does not replace provider drivers or claim complete process/host independence.

A fresh production read discovered another real fleet stoppage: at 05:28:43Z a lifecycle BULK_DELETE encountered `ErrExecutionGated` while its panel was retiring. The worker treated this admission refusal as an internal failure and closed the global gate. The job later became OBSOLETE. At 05:55Z public shares returned Direct32, Residential0; healthy residential proxies were available. The new release prevents this typed refusal from closing the fleet. Deployment preserves the already-closed administrative gate; authenticated Resume remains required for the existing incident.

## Implemented boundaries
- Every queued client origin uses typed panel COOLDOWN/QUARANTINED handling. Explicit native transport errors retain their provenance; SQL, secret-store and unknown failures do not become panel errors through the runtime circuit.
- A changed execution scope produces a durable 30-second policy deferral without refunding the already-consumed claim attempt/authorization. It does not widen a gate, rearm a scope, clear quarantine or repeat a failed/obsolete job.
- Native runtime acquisition/mutation waits honor cancellation before entry. A timeout while waiting does not imply a POST occurred. Actual ambiguous mutations retain native readback before retry.
- Manual client inventory validation checks the complete list, rejects absent/null/non-array/malformed records and duplicate identities, including errors after the target record.
- Below Desired, a full fresh provider ceiling can retire exactly one oldest expired, currently managed server. Account locks serialize admission; active creation, retirement, pending backfill, stale/future/unknown/zero capacity and lost ownership block this action. Desired remains unchanged and the scheduler owns backfill.
- Expiry decisions re-read and lock current account, droplet, ownership, expiry and exact provider target. State/event writes are atomic. Provider-lock policy remains an explicit independent retirement reason. Account-rule selection and final admission revalidate managed ownership.
- Provisioning retry reads the step-qualified next_retry_at and propagates failed SQL reads. A failed read cannot authorize another provisioning attempt.
- Lifecycle processing has six bounded lanes, one per account, a 90-second cooperative item deadline, bounded discovery and rotation of offered accounts. A canceled handler keeps its lane until it returns; no abandoned task overlaps its replacement.
- Failure counts and next-retry timestamps commit atomically. Ledger read errors refuse work rather than bypassing backoff.
- Heartbeats distinguish client scans, attempts, durable successes, remote isolation, policy deferral and policy-gated idle state. Readiness observes client progress and overdue lifecycle lanes; scanner liveness is not a claim that Output is populated.

## Verification
Run tools/modular-recovery-acceptance.sh. It uses only the isolated bulk_test PostgreSQL database for fixtures, with real migrations, HTTP fault servers and Go race checks. It includes:
- all-origin HTTP failures/timeouts and restart-persistent cooldown/quarantine;
- committed POST timeout with exact readback and one POST only;
- scope revocation between Claim and mutation, explicit operator pause preservation;
- malformed/duplicate inventory and native lock deadline/provenance checks;
- concurrent exact-one capacity claim, stale discovery, altered target, lost ownership, atomic event rollback;
- existing provider receipt replay, local completion failure/rollback, late CAS and no duplicate deletion;
- independent/fair lifecycle account lanes, no overlap after cancellation, durable backoff and readiness states;
- account-rule ownership/transaction tests, authenticated capacity/output regression and full Go suite.

Original failed fixture/review results remain in /root/backups/dob-modular-recovery-20261007. Acceptance and source-snapshot JSON distinguish test success from production proof.

## Recovery limits and next scope
Global client concurrency remains one with its existing advisory lock. API/worker have separate processes, but worker modules still share process, memory and DB resources. A driver that ignores cancellation is marked stalled and retains its lane; this stage does not forcibly kill its goroutine. Provider outages, billing/trial restrictions, failed ownership proofs, quarantine, expired/exhausted budgets and explicit operator pause are not bypassed.

Further process/resource partitioning, independent proxy-health evidence, cross-window queue fairness, and supervised restart policies need separate source-grounded fault acceptance. No indefinite uptime or automatic repair of every unknown bug is claimed.

## Deployment and rollback
No schema or static-UI change. Build API/worker from a clean detached checkpoint; back up binaries/manifest and configuration hashes. Verify running binary hashes, readiness, module metadata and natural lifecycle progression. Preserve all account, routing, proxy, profile, budget and execution-gate settings. Rollback restores both binaries/manifest; if the gate changed concurrently, do not overwrite user intent or blindly start an older worker.

## Deployment result
API/worker caf3f0de76d142b7e8ded2923a429b00fd4ba50c deployed at 2026-10-07T06:14:52.561161+00:00; actual process hashes, all readiness checks, unchanged static/schema and configuration equality verified. The existing client gate remains closed, so new client creation and Residential Output recovery await authenticated Resume. UpCloud progressed from the diagnosed capacity deadlock to exactly one owned retirement intent; native provider deletion is still unconfirmed after a request timeout. No successful replacement or full user traffic is claimed. See the latest acceptance JSON before continuing.
