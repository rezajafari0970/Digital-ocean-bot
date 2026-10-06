# Optional application of account build rules

User correction, 2026-10-06: existing servers must NOT rotate merely because account build rules were saved. Account Edit has a saved, default-OFF "Apply rule changes to existing servers" control. No account is enabled by migration or deployment.

| Edit on Save | OFF | ON |
|---|---|---|
| Lifetime, region, plan, image or fallback changed | New builds only; retain existing expiry | Queue older build revisions for sequential replacement |
| Desired increases | Scheduler fills only deficit | Same; no new replacement rollout |
| Desired decreases | Natural expiry converges to target | Retire only exact excess, earliest expiry first |
| Spacing / concurrency only | Update scheduling | Update scheduling; no new rollout |
| No effective change | No new work | No new rollout or revision |

Combined spec + Desired edits independently retain both intents: growth fills deficit before further retirements; shrink removes excess first. Enabling the switch alone does not create a rollout. Previously explicitly requested work may continue across a Desired-only edit; turning OFF cancels unstarted work.

Implementation:
- Migration 158 adds account_rule_application and deployment build_rules_revision. Existing rows are revision0; no expiry updates and no automatic rollout.
- Save compares canonical JSONB build specs under deployment-admission lock. Desired/cadence are excluded. Rules revision, account settings, auto profile, and rollout intent commit together.
- New deployment reservations capture the revision inside the same lock/transaction that captures the immutable rules. Already admitted builds finish with their captured rules; if obsolete they become rollout candidates.
- Worker checks due accounts every5seconds. One owned resource is claimed per account. Provider identity, pending work, fresh capacity, create denial, and network conditions remain gates.
- Hard Desired ceiling is preserved: ONE delete-first retirement, provider-confirmed absence, immediate backfill scheduling without a second Build-spacing delay, then readiness before NEXT retirement. There is no implicit surge/cost increase.
- The interval is persisted, inclusive random configured minutes. No catch-up burst. Provisioning readiness can make actual spacing longer.
- Earliest expiry includes EXPIRING resources when reducing Desired, preserving healthy servers if excess is already unavailable.
- Save OFF restores unstarted RETIRING claims to their original READY/EXPIRING state without rewriting expiry. It shares locks with the durable lifecycle admission boundary. Stale lifecycle reads are fenced. Once deletion is admitted, its possibly ambiguous outcome must reconcile; OFF cannot undelete a cloud VM.
- Ordinary expiry and existing confirmed-delete recovery remain authoritative. Account deletion cascades the new control row.
- A failed/unready replacement prevents the next requested retirement. Provider/cloud latency is not a zero-time creation guarantee.

Source review:
- AI OS orchestrator job account-rule-application-20261006; server OpenAI API plan and source review.
- Initial API plan suggested replacement-first; rejected because user/project Hard Desired explicitly requires no implicit extra server.
- Source review found unstarted-OFF cancellation and earliest-EXPIRING selection gaps; both fixed with regression tests.

Acceptance:
- Isolated PostgreSQL with all158migrations, shared DO/Vultr/UpCloud identity fixtures.
- Actual account PUT/list, default OFF, 60–90 to150–160minute future-only lifetime, Desired15→16 no rotation, 15→14 exact shrink.
- Duplicate Save, concurrent claims, rollback, persisted restart, spacing, denied/stale provider, OFF cancellation and stale-worker fence.
- Browser fixture exercises actual API on mobile/desktop, Save/readback/reload.
- Full Go suite and focused race tests. Fixtures do not call real cloud create/delete APIs.
- Production status and clean build hashes recorded separately after deployment; cloud provider end-to-end replacement is not claimed by fixture tests.

Rollback:
1. Save OFF for active rule application controls through the current API, which cancels unstarted intent.
2. Reconcile already-admitted retirements and pending/unknown provider operations with this version.
3. Restore backed-up binaries/static if necessary. Keep migration158 and evidence until work is settled; old binaries ignore the additive schema.
4. Do not down-migrate or erase in-flight provider evidence to force rollback.
