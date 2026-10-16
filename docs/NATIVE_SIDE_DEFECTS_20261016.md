# Three native side defects — 2026-10-16

User explicitly authorized these three fixes without OpenAI API. No API council is claimed; no paid requests are made. The separate long advisory-lock/output-coverage work remains deferred. This branch starts from the last canonical checkpoint and imports only the three relevant baseline regressions; the unresolved output-lock reproduction remains preserved on feat/output-controls-20261016.

## Requirements and invariants
- Canceled or expired readers must return promptly while another HTTP read owns the session; they cannot reuse cached success. A canceled loader must not cache its own cancellation for future callers.
- Preserve one reader and consistent raw/index ownership. Pre-read invalidation and read use context-aware ownership; post-mutation invalidation remains unconditional, even after cancellation.
- A policy SQL failure must be observable. Missing policy and explicit disabled policy remain no-ops.
- Only a well-formed, authoritative semantic mismatch may initiate structural repair. SQL errors, missing public key and malformed observations must not initiate a native update. A real target mismatch still converges; client-preservation and mutation/config-lock ownership remain enforced.
- Do not change Xray, provider policy, credentials, output toggles, freshness (15s), route proof (60s), advisory locks, or concurrency budgets.

## Implementation decisions
Session network serialization is retained, with context-aware TryLock waiting using the same style as existing runtime/auth guards. Snapshot and index lookup now share one ownership interval, fixing an invalidation race. FreshSnapshot/FreshRawInbound combine pre-read invalidation and reading; ordinary Invalidate remains for every identified after-write cleanup. This avoids a broader single-flight/generation rewrite.

Global policy query handling separates ErrNoRows, query failures and disabled state in that order.

Desired verification checks public-key availability before returning a repairable mismatch, validates both expected payload and observation shape before authorizing repair, and propagates every non-mismatch failure. This prevents the existing any-error-to-update fallback. Complete mismatched observations retain the normal structural update path.

## Evidence and current boundary
The old code failed all three imported baseline tests against isolated PostgreSQL and real local HTTP fixtures. Corrected focused tests pass, including malformed/incomplete REALITY, malformed clients, missing key plus real drift, metadata SQL failure, cached cancellation, blocked readers, deadline expiration, live-reader recovery and concurrent invalidation/index pairing. Matching and real structural mismatch controls pass. Broad race/full/static verification and rollout results are recorded in the development job; do not infer they passed from this design note.

Rollback release: 90fe8c3c71e513c0be7766cdc7a3a238ff8b2003. No database migration. Production faults are not injected; all adversarial failures use the isolated test database/native HTTP fixtures.
