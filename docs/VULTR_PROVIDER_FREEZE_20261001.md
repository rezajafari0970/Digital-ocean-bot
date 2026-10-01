# Vultr Provider Freeze — 2026-10-01

## Frozen baseline

- Branch: `checkpoint/final-e2e-20260929`
- Source revision: `7b776a28a980f35057a7a25303b482bfc3073615`
- Acceptance UTC: `2026-10-01T22:10:53Z`
- API service: active
- Worker service: active
- Account runtime: READY
- Provider state: ACTIVE
- Provider error: none

## Production acceptance evidence

At the acceptance snapshot:

- Desired servers: 15
- Local managed servers: 15
- Vultr provider inventory: 15
- Provider resource-registry servers: 15
- Unresolved operations: 0
- Capacity source: `vultr_api_lower_bound`
- Proven lower bound: 17
- Exact account limit: unknown
- Capacity probe in flight: false
- Successful provider SSH-key cleanup markers observed: 10

The historical `Proven >= 17` value is valid evidence that at least 17 instances existed concurrently before the Desired-ceiling race fixes. It MUST NOT be rewritten downward merely because current Desired or current inventory is 15.

## Frozen invariants

1. **Hard Desired ceiling**
   - New rotations must not intentionally create above Desired.
   - Admission occupancy is `max(local managed + unmaterialized active deployments, fresh provider in-use)`.
   - A provider ID assigned before local droplet materialization still consumes a slot.

2. **Provider-confirmed delete before backfill**
   - Local deletion alone does not free a build slot while fresh provider inventory still reports the instance.
   - Scheduler, StartDeployment, and final PreCreateCheck independently enforce the ceiling.

3. **Mutation ambiguity**
   - POST create is never blindly retried after 429, 5xx, timeout, or ambiguous transport outcome.
   - Ambiguous create remains an unknown operation until identity reconciliation.
   - Delete transport ambiguity is reconciled by provider presence/absence.
   - Delete 404 is idempotent success.

4. **Capacity semantics**
   - Only an account-level create-capacity rejection may prove exact saturation.
   - Region shortage, generic rate limiting, maintenance 502, and transport failures never prove account capacity.
   - Proven lower bound is monotonic.
   - Desired ceiling stops capacity discovery; the engine must not create Desired+1 solely to discover a limit.

5. **Refresh isolation**
   - Fast provider observation is Account + Inventory + Capacity.
   - Fast observation must not depend on Plans/Regions/OS catalog endpoints.
   - Catalog failure must not stale an otherwise successful instance inventory refresh.

6. **Vultr API contracts**
   - Create Instance uses `ssh_key_ids`, never legacy `sshkey_id`.
   - `user_data` remains base64 encoded for Vultr.
   - Regions, Plans, OS, and Instances follow pagination `meta.links.next`.
   - GET 429 may retry; mutation requests do not automatically retry.

7. **SSH/provisioning**
   - Each deployment receives an independent SSH identity.
   - Public-key fingerprint may be recorded; private-key material must not be logged.
   - Initial SSH timeout/refused is retryable readiness behavior.
   - Final provisioning attempt terminalizes immediately; no extra backoff after MaxAttempts is exhausted.
   - Retry-limit errors preserve step, attempt count, and root error.
   - Provider SSH key is cleaned up after confirmed server deletion; successful cleanup is recorded.

8. **Lifecycle**
   - Due-item selection is account-fair.
   - A failed replacement cannot permanently pin an EXPIRING server.
   - At the hard Desired ceiling, rotation is delete-one-first then provider-confirmed backfill-one.
   - Multiple EXPIRING items do not simultaneously force replacement attempts.
   - Lifecycle rotation must converge without unresolved operations.

9. **Registry semantics**
   - Provider inventory rows are `type=server`.
   - Local lifecycle mirror rows are a separate namespace and must not be double-counted as provider servers.
   - Dashboard/provider counts use the provider-server namespace.

## Regression gates

Before accepting changes that touch shared provider, scheduler, capacity, lifecycle, provisioning, recovery, resources, or workflow code, run:

```bash
go test ./internal/providers/vultr ./internal/droplets ./internal/app ./internal/capacity ./internal/scheduler ./internal/provisioning ./internal/resources -count=1
go test ./...
```

The Vultr-specific suite must continue covering:

- current SSH-key request contract;
- create 429 / 502 / client timeout with no blind mutation retry;
- delete 404 / 502 behavior;
- ambiguous-create identity reconciliation and duplicate prevention;
- account capacity vs region capacity vs rate-limit/transport taxonomy;
- fast observation isolation from catalog outages;
- Regions/Plans/OS/Instances pagination;
- Ubuntu normalization;
- hard Desired and materialization-gap admission;
- lifecycle rotation and failed-replacement recovery.

## Freeze rule for DigitalOcean work

DigitalOcean implementation may share abstractions, but it must not weaken any invariant above. If a shared-code change causes a Vultr regression, the change is not accepted until the Vultr suite and production invariants are restored.
