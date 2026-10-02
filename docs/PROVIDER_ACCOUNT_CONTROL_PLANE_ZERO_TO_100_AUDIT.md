# Provider / Account Control Plane — Zero-to-100 Audit

Date: 2026-10-02

## Scope
1. Account state authority
2. Provider observation authority
3. Capacity authority
4. Scheduler admission
5. Mutation idempotency
6. Recovery/reconciliation
7. Lifecycle/desired capacity
8. Provider parity
9. Concurrency/fault handling
10. Observability/production verification

## Confirmed strengths
- Provider-neutral Driver/Factory/Compute/Observation contracts.
- All provider HTTP clients enter through AccountRuntime/Provider Network Runtime.
- StartDeployment is the authoritative create admission: account advisory lock, state recheck, desired/capacity recheck and durable deployment reservation share one transaction.
- Capacity snapshots are freshness-gated. DigitalOcean has API-known hard capacity; Vultr uses API-derived lower-bound/saturation/probe evidence.
- Provider create outcomes distinguish accepted/rejected/ambiguous.
- Create identity tags plus reconciliation prevent blind duplicate create after ambiguous outcomes.
- Delete is provider-idempotent (404 is success).
- Scheduler is a prefilter; StartDeployment rechecks authority.
- Lifecycle has account-scoped retirement/replacement locks and hard-desired handling.
- DigitalOcean/Vultr parity tests and provider contracts exist.

## P0 findings discovered in this audit
1. operations.lock_version existed in schema but SQLStore.Update did not compare the expected version. Multiple workers could overwrite operation state.
2. DELETE_DROPLET unknown operations could remain unknown indefinitely. Recovery observed that the resource still existed, wrote unknown again, returned nil, and therefore bypassed FailureStore backoff. Five live operations had looped for about 64 hours with roughly 300 attempts.
3. runtime_status READY had multiple owners. Network/sticky/lifecycle success paths could overwrite provider blocking state. Live evidence showed a Vultr TOKEN_INVALID account with runtime_status READY.
4. Provider refresh rechecked snapshot freshness after acquiring its advisory lock but reused provider state/error values read before waiting on the lock.
5. A Vultr timeout test contained an unsynchronized test counter, preventing the full provider race suite from being trustworthy.

## Corrections
- Operation now carries LockVersion. SQLStore.Get reads it and Update is a compare-and-swap on lock_version. Successful updates advance the caller's version; stale writers receive ErrOperationVersionConflict.
- DELETE_DROPLET recovery now reissues the idempotent provider delete when the resource still exists. Temporary provider/network errors are returned so FailureStore backoff applies; successful reissue moves the operation to verifying.
- Network identity, sticky recovery and lifecycle may set READY only while provider_state=ACTIVE and provider_error_state is empty.
- Migration 000110 adds a database backstop preventing READY/provider-state divergence and normalizes existing inconsistent rows.
- Provider refresh re-reads provider state/error after acquiring its advisory lock.
- Vultr timeout test counter is atomic; full provider race suite is clean.

## Live pre-fix evidence
- active accounts: 5
- provider_state != ACTIVE: 1
- provider_error_state present: 0
- stale provider snapshots: 1
- unknown operations: 5
- running operations: 0
- duplicate account/idempotency keys: 0
- accounts over desired: 0
- managed resource orphans: 0
- all 5 unknown operations were DELETE_DROPLET with resource IDs, about 64h old, attempts around 300
- inconsistent account: Vultr TOKEN_INVALID + runtime_status READY

## Verification before promotion
- go test -race: jobs, droplets, app, scheduler, capacity, providers, DigitalOcean, Vultr, VultrConsole PASS.
- go test ./... PASS.
- go build ./cmd/api ./cmd/worker PASS.
- git diff --check PASS.
- migration 000110 executed against the live production schema inside BEGIN/ROLLBACK: PASS; it identified exactly one existing READY/provider inconsistency.
- operation CAS behavior validated against production schema inside BEGIN/ROLLBACK: fresh CAS succeeds and stale CAS updates zero rows.

## Release invariants
- READY => provider_state=ACTIVE and provider_error_state empty.
- stale operation writer cannot overwrite a newer operation state.
- account+idempotency_key remains unique.
- scheduler cannot bypass StartDeployment's authoritative admission.
- ambiguous create never blindly creates a second server.
- unknown delete either converges through idempotent delete+verify or returns an error and receives backoff.
- provider mutation remains behind AccountRuntime mutation/egress guards.
- capacity evidence must be fresh at authoritative create admission.
- desired capacity remains a hard creation ceiling.

## Production completion evidence
Production promoted to commit 870750aa3d0fa29c2a5e361551da1e8fce6b5b0f. Runtime version, build manifest, API hash and worker hash all match; API and worker are active.

Live database migration metadata reports 111 migrations with latest 000110_account_runtime_provider_invariant.

Post-promotion evidence:
- TOKEN_INVALID Vultr account is now runtime_status=PROVIDER_TOKEN_INVALID rather than READY.
- duplicate idempotency keys: 0.
- accounts above desired capacity: 0.
- managed resource orphans: 0.
- five historical DELETE_DROPLET unknown operations belong to a LOCKED DigitalOcean account. They cannot be safely mutated until the provider account permits access.
- all five historical unknown operations now have worker failure/backoff rows; observed retry horizon was about 50 seconds during verification and operation attempt counters did not continue the prior 30-second growth pattern.
- the locked account and its stale snapshot remain intentionally fail-closed; this is external provider state, not a local convergence defect.

Rollback artifact captured before promotion at /opt/digital-ocean-bot/rollback-provider-870750a.
