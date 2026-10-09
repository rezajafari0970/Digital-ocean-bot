# Next phase: per-VPS destination health and reversible admission

Status: design boundary only; not implemented, deployed or authorized for automatic promotion by this checkpoint. User authorization to perform engineering remains in place. No additional permission is needed merely to implement/test a bounded solution. The present blocker to activating an exclusion is missing supported code and its correctness/canary evidence, not missing API credit.

## Requirement and evidence

RES-001 and the existing strict residential contract remain authoritative. A residential gateway can pass a generic204/ipify check and fail GPT/BrowserLeaks from one VPS, while working from the controlhost. Therefore central generic status must neither establish target health nor become an automatic global blacklist.

The latest bounded confirmation found GPT failures3/3 via Vultr panel4dd711d3-00fe-4003-bcbf-51a2a4396e4b and gateway8652799c-7d5a-4a9a-8d86-7d1f519b31dc; matched controls6/6 succeeded. This is a time-bounded observation. Revalidate the live panel lifetime, endpoint configuration, ownership and destination failures before any later trial. Do not reuse this as indefinite eligibility.

## Implementation boundary

1. Keep immutable observations keyed by panel, gateway identity/configuration version, exact target, source path and measurement time. Unknown, expired, interrupted and failed samples are distinct. Store safe error classes and timings, never proxy/client credentials or complete URIs.
2. Add an explicit, versioned **per-panel** admission policy with a short canary deadline and exact restore record. It must not rewrite global proxy enablement or the permanent fleet performance profile. Begin with reviewed manual admission; automatic quarantine is a separate gated step.
3. Load performance assignment and local admission policy as one coherent desired snapshot. A single eligibility calculation must feed TCP/UDP candidates, fallback, observers, advertised residential availability and native acceptance. No excluded endpoint may remain reachable through an alternate pool lane. Empty pool keeps RES blocked while preserving unrelated DIRECT and exact allowed client probes.
4. Serialize policy writes against existing performance/routing ownership, retain monotonic desired/applied generations and reject stale ACKs. Controller mutation remains under existing panel locks. SQL-only operators must never hold a global lock while waiting for panel I/O.
5. Avoid coupling every transient probe to a config fingerprint/restart. Use explicit state transitions, a recorded hold/cooldown, bounded rechecks and native proof before readmission. Measure connection disruption during any native apply; do not claim existing sessions are drained without evidence.
6. Recovery must survive worker/process restarts, expired or unreachable panels and partial application. Restore exact prior local policy; a deleted panel is different from an unreachable one. A deployment/rollback compatibility barrier must prevent an older consumer from silently ignoring an active or still-applied local exclusion.
7. UI/status should report generic endpoint health separately from per-VPS destination observations and their age. Global healthy is never renamed to imply all targets work.

The optional Config.ExcludedProxyIDs proposal discussed in the council is not itself a complete solution: existing tune_publish promotes one candidate to the whole fleet. Adding a JSON field alone would not provide a durable per-panel policy or correct publication/recovery ownership.

## Gates before one pilot

- New scoped development job and deterministic risk routing; independent architecture/adversarial/test review against exact code.
- Validation, authorization, CAS/idempotency, cross-panel isolation, stale-version/generation, deadline/crash/partial apply, downgrade refusal and exact restoration tests.
- Legacy absent-policy JSON/fingerprint compatibility; no health-flap restarts; zero/one/multiple eligible pools; actual supported Xray versions; TCP/UDP/fallback/observer and strict allowlist proofs.
- Current source/runtime provenance and the supported release path. The detached bootstrap failure is preserved; no full readiness claim or bypass of branch/source gates.
- Freeze exact panel, full gateway ID, observed versions, sufficient remaining pool, request/concurrency budget, duration and numerical failure/latency criteria before pilot measurements. Diagnostic chaining is evidence about a VPS path; native pool selection and end-to-end RES need separate proof.
- One timed exclusion-only pilot retaining13/80; cancel on any route leakage, wrong scope, generation mismatch, missing native proof or predefined reliability regression. No automatic extension, global publication or promotion from old observations.

## External evidence still required

Actual Android/v2rayNG latency needs a physical client trace. Actual ad show-rate and creative quality need the app repository/SDK version and load/show/impression/error callbacks (and, where available, destination/network traces). StaticHEAD/JS success is not ad fill or show-rate. Upstream provider logs/limits are needed to distinguish gateway rejection, overload, source restrictions and remote TLS/transport failure; the current measurements do not establish which internal vendor mechanism caused the observed errors.

## Continuation

Canonical HANDOFF and the structured health acceptance file preserve the exact boundary. Engineering on the next phase has not begun at this checkpoint. The development agent is inactive after its final message; normal application workers continue. Never present an expired progress heartbeat as continuing AI work.
