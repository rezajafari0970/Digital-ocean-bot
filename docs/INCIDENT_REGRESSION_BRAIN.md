# Historical Incident / Regression Brain

Commit `9a57a8921e235351c57943af23724dc7e119474f` — **10 durable incident/regression classes**, **198 candidate regression-related commits** from recent history.

This brain teaches a fresh session what previously went wrong, the durable lesson, and the guardrails that must not regress. Commit associations are lexical navigation candidates, not automatic proof of causality.

## `INC-001` — provisioning
- Failure: duplicate/competing installer execution
- Lesson: Long-running install work needs one controlled execution owner; in-process locks alone are insufficient across processes.
- Prevent: single lifecycle owner, per-resource coordination, observable install diagnostics

## `INC-002` — proxy_network
- Failure: shared account egress/proxy identity
- Lesson: Account network identity must remain isolated; shared proxy state is a regression.
- Prevent: per-account isolation, fail-closed required proxy

## `INC-003` — output
- Failure: stale output visibility / approximate expiry
- Lesson: Use persisted exact visibility/expiry semantics and apply them consistently to direct/share output.
- Prevent: visible_until, canonical validity filter, avoid coarse TTL-only logic

## `INC-004` — output
- Failure: high-frequency refresh causing memory pressure/OOM risk
- Lesson: Keep Output fast but use lightweight/separated refresh rather than unbounded expensive refresh work.
- Prevent: bounded refresh work, live API path, avoid SSH aggregation

## `INC-005` — providers
- Failure: DigitalOcean-specific assumptions leaking into generic provider paths
- Lesson: Generic lifecycle must not validate/catalog/reconcile with DO-only assumptions.
- Prevent: provider-neutral contracts, provider-specific drivers

## `INC-006` — vultr
- Failure: unknown Vultr capacity treated as hard blocker
- Lesson: Unknown API limit is not the same as zero capacity; authoritative capacity observation is provider-specific.
- Prevent: LimitKnown semantics, console capacity observation when needed

## `INC-007` — vultr_console
- Failure: noVNC/WebSocket path mismatch / connecting state
- Lesson: Reverse proxy must preserve the supported WebSocket upgrade path and interactive session authorization.
- Prevent: websockify path compatibility, fresh console ticket/session, interactive challenge visibility

## `INC-008` — database
- Failure: schema/source drift or migration mismatch
- Lesson: Verify live migration level/checksums before schema-sensitive work; do not infer live schema solely from source migrations.
- Prevent: schema_migrations checksums, LIVE_DB_BRAIN, migration verification

## `INC-009` — production
- Failure: assuming Git HEAD equals deployed production
- Lesson: Service health does not prove revision identity.
- Prevent: /version stamp, artifact manifest hashes, production revision verifier

## `INC-010` — continuity
- Failure: new chat reconstructing project from stale/partial history
- Lesson: Use canonical repo plus commit-pinned knowledge brains and acceptance gates.
- Prevent: NEW_CHAT_ENTRYPOINT, Project Brain, handoff acceptance gate

