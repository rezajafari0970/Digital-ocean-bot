# Proxy Control Plane Zero-to-100 Audit

Audit baseline: canonical HEAD 06ca8c74c384ca6a29fc706b16351477fc0c6d4b.
Production evidence: running manifest/binaries are internally consistent at e532874ad79e6971a94d8267edd6244b60ec1267, but production does not match canonical HEAD.

## Target architecture
One account-scoped Provider Network Runtime is the only provider egress boundary. For proxy-required accounts, a transport identity is a versioned tuple: account + provider + active proxy + proxy endpoint/credential semantics + sticky/fallback identity. Every material identity transition must atomically invalidate the prior generation before new provider mutation is admitted.

The control plane should expose explicit commands instead of scattered table writes:
- AdmitRuntime
- RecordObservation
- RotateActiveProxy
- ChangeProxyDefinition
- ChangeNetworkProfile
- RefreshIdentity
- MarkRecovery/Ready
Each command owns locking, generation invalidation, identity reset, status update and audit evidence.

## Confirmed strengths
- Provider runtime is centralized through buildProviderNetworkRuntime.
- Proxy-required path fails closed.
- SQLStore Acquire uses transaction + FOR UPDATE for half-open lease.
- Generation bump is atomic.
- Passive observations can be generation-gated.
- Mutation generation guard exists.
- E2E 407 -> circuit -> recovery -> stale rejection exists.
- go test -race for proxycontrol/network/app passes.

## P0 correctness gaps
1. Generation invalidation is not coupled to all transport identity changes. BumpGeneration is only called from proxy_control_plane.go, while admin/profile/pool/proxy-definition writes can change active proxy or endpoint semantics independently.
2. ensureActiveAccountProxy chooses candidate before the account advisory lock and does not revalidate pool membership/health under the lock before committing the switch.
3. SQLStore transactional behavior (Acquire, BumpGeneration, ApplyObservationForGeneration) has no direct PostgreSQL-backed tests.
4. State ownership is fragmented across proxy_runtime_state, network_profiles/account_proxy_pool, account_network_identities and proxies; multiple code paths write these independently.
5. Production is behind canonical HEAD, so source-level guarantees are not yet production guarantees.

## P1 reliability/consistency gaps
6. Migration 000057 DB enforcement for adapter identity state is disabled. Generic/sticky invariants rely on every application write path being correct.
7. Several proxy/identity/status DB writes intentionally ignore errors. Telemetry-only writes may remain best-effort, but authoritative identity/rotation transitions must not.
8. Proxy definition edits reset identity state but do not centrally invalidate runtime generations for all affected accounts.
9. Manual proxy health test writes proxies and account_network_identities directly, outside one explicit proxy-control command.
10. account_runtime_state and proxy_runtime_state are separate circuits, but admin runtime/dashboard surfaces primarily expose the former; proxy circuit/generation/lease visibility is incomplete.

## P2 architecture debt
11. Health exists at global proxy row level and account/provider runtime level; authority and propagation rules need explicit documentation.
12. Sticky rotation, pool rotation, identity collision and runtime generation are separate flows. They need one transition coordinator, not one giant table.
13. Runtime generation key includes proxy_id; switching proxy changes the key. A separate account-level transport epoch is preferable for invalidating any runtime captured before an active-proxy transition.
14. No durable transition/audit ledger records why generation changed, old/new proxy, reason, actor and resulting identity.
15. Council artifacts use a shared latest-council.json path and are not safe for simultaneous unrelated council runs; this is AI-OS debt discovered during this audit.

## Recommended end-state
Keep specialized tables but establish one authoritative transition service:
- account_transport_state: account/provider-level monotonic transport_epoch, active_proxy_id, transition_state, reason, timestamps.
- proxy_runtime_state: health/circuit state for the concrete account+proxy+provider transport.
- account_network_identities: observed/sticky geo identity only.
- account_proxy_pool: configured candidates only.
- proxies: proxy definitions/global observations only.

All active-proxy/profile/proxy-definition transitions go through the coordinator under one account-scoped transaction/advisory lock. The coordinator increments transport_epoch on every material transport change. AccountRuntime captures transport_epoch and mutation checks compare the account-level epoch, not only the old proxy-specific row generation.

## Required invariants
- proxy_required && no admitted transport => provider mutation denied.
- every material transport identity change increments account transport_epoch exactly once.
- runtime.epoch != current epoch => mutation denied.
- stale observation cannot change authoritative current state.
- at most one live half-open probe lease per account+proxy+provider.
- active proxy must be enabled and eligible at commit time.
- proxy switch + identity reset + epoch bump commit atomically.
- failed transition cannot expose mixed old/new identity.
- account A transitions cannot mutate account B state.
- proxy-definition change invalidates every affected account runtime.
- authoritative transition DB errors are never ignored.

## Test strategy
1. PostgreSQL integration tests for SQLStore Acquire contention, lease expiry, generation/epoch bump and stale observation.
2. PostgreSQL concurrency test for two simultaneous pool rotations; exactly one committed active proxy and monotonic epoch.
3. Proxy definition edit with two affected accounts invalidates both epochs.
4. Profile switch direct<->proxy_required invalidates runtime.
5. Race: runtime captured before proxy switch is rejected after switch.
6. Race: selected pool candidate becomes unhealthy/disabled before lock; transaction reselects or fails closed.
7. Fault injection between switch/reset/epoch update proves atomic rollback.
8. Cross-account isolation with concurrent rotations.
9. Existing 407/circuit/hysteresis E2E retained.
10. Provider parity: DigitalOcean/Vultr use the same runtime boundary and mutation guard.

## Rollout sequence
Phase A: tests and observability only; no behavior change.
Phase B: introduce account transport epoch + transition coordinator behind current behavior.
Phase C: route pool/profile/proxy-definition transitions through coordinator.
Phase D: switch AccountRuntime mutation guard to account transport epoch; dual-read/compare during migration.
Phase E: remove scattered authoritative writes or make them coordinator-private.
Phase F: deploy canary, shadow old/new decisions, fault injection, then production promotion.

## Immediate next batch
Do not start with schema redesign. First prove the current races:
- add PostgreSQL-backed SQLStore tests;
- add pool rotation transaction race test;
- add tests demonstrating generation invalidation gaps on proxy switch/definition edit;
- reconcile the unfinished dirty concurrency test;
- expose proxy circuit/generation separately in admin observability.
Only then introduce the account transport epoch migration.

## Completion status — 2026-10-02
The six-stage completion train was executed through production promotion.

- B1: account_transport_state migration and dual proxy-generation + transport-epoch mutation guard implemented.
- B2: active proxy selection moved under account advisory lock/transaction; candidate eligibility is selected under lock and switch + identity reset are atomic.
- C: DB invariant triggers invalidate epoch on network profile and proxy-definition changes; password-only credential changes explicitly invalidate affected account epochs.
- D: runtime API distinguishes provider circuit from proxy circuit and exposes proxy health/generation/lease plus account transport epoch/active proxy/reason.
- E: race suites, provider parity, full regression, existing 407/circuit/stale-runtime E2E, production-schema migration dry-run with rollback, build verification, and controlled worker restart passed. A destructive production DB outage was intentionally not injected.
- F: production upgraded to commit c007e318ec41769c88e2938af0c2e59e0a3dac43. Runtime commit/manifest/binary hashes match; API and worker are active. Live migrations 000108 and 000109 are applied. Live invariant check: 8 proxy-required accounts, 0 epoch/proxy mismatches, 0 missing transport-state rows, 0 open proxy circuits at verification time.

Rollback artifact captured before promotion at /opt/digital-ocean-bot/rollback-proxy-c007e31.
