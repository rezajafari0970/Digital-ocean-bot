# Permanent residential publication — 2026-10-05

This supersedes the single-canary/timed-only UX limits in RESIDENTIAL_PERFORMANCE_HANDOFF_20261005.md. The exact native settings and residential domain scope are unchanged.

## User workflow
Residential → Performance & rollback → Publish settings.
New form defaults:
- Duration: Permanent — no automatic rollback.
- All current and future servers checked; current server checkboxes all selected.
- Select all and Clear selection are visible. Unchecking a server/clearing selection disables future inheritance, making the request selected-only.
- Timed test remains available; its minute field is enabled only in timed mode. Timed selections still require fresh healthy runtime eligibility.
- Preview publication fetches a fresh routing revision, freezes the chosen config/scope into the request, and shows permanent/timed duration, current target list and future inheritance.
- Publish permanently records desired policy; per-server proof status is separate from publication success.
- A running or kept selected profile can be published to all current/future servers with a separate reviewed button; existing baselines/config are preserved.
- Manual Rollback all settings stops future enrollment atomically, then restores all assigned targets. Offline/unverified/gated targets remain pending.
- Change to a different profile still requires rollback of the current one; a second open profile cannot hide rollback ownership.

Settings are shared by all existing/future users through server configuration. Existing DIRECT/RESIDENTIAL client classes, identities and credentials are preserved; this does not reroute DIRECT users' Ads traffic through residential proxies or broaden google@ads + temporary BrowserLeaks.

## Persistence and lifecycle
Migration 152 adds duration_mode and publish_scope, preserving old timed records. Permanent deadline is NULL with a database constraint, not a large timeout. KEPT means saved permanent intent for new permanent publications; only target APPLIED plus fresh matching runtime generation is actual verification.

Scope selected stores reviewed IDs (up to 1000). Scope fleet is resolved on the server and has no fixed total target cap. The worker admits up to 64 missing current/future panels every five seconds under the same PostgreSQL advisory transaction lock as publish/rollback. All enabled tracked panels of enabled ACTIVE accounts that are not retiring/deleting/deleted/expired can be assigned, including setup/unverified panels. Existing routing gates/readiness decide when mutation can run; no operator gate is reopened.

The permanent fleet policy remains active when old servers retire and admits replacements. Baselines are recorded separately immediately before each target's first mutation. Auto-enrollment does not bump administrative version; expected_version and idempotency operation hashes continue to protect explicit user actions. Mode/scope JSON fields are omitted for old callers, preserving pre-152 replay hashes.

Status includes complete aggregate counts and at most 200 prioritized target detail rows, avoiding an ever-growing retirement history payload. Fresh proof flags show VERIFYING instead of treating stale APPLIED receipts as current success.

## Acceptance
AI OS/OpenAI API plan and Development Orchestrator PLAN→IMPLEMENT→TEST→VERIFY→CHECKPOINT completed. Full Go, race/PostgreSQL, installed Xray TCP/UDP fault and profile rollback tests passed. Added tests cover multi-server permanent publication, absent deadline, selected-only scope, restart/re-enrollment, old timed migration/baseline preservation, future admission versus rollback, response-loss replay, closed-gate preservation and unsafe schema downgrade rejection.

Mobile/desktop browser tests passed for Select all, Clear selection, Permanent/no timer, future inheritance, publication with a failed pending server, lost response after commit, reload, and rollback stopping inheritance. Legacy timed and bulk-proxy browser flows also passed.

Production UI read-only acceptance displayed 27/27 selected current servers with future inheritance checked. No global publication was activated by this task.

Live permanent selected canary:
- Server a962430f-5760-4df2-9cb0-1c0cc2c3fd5d / 70.34.209.142:2053.
- Publication c2403121-d876-4cfb-aaeb-64777264e564.
- KEPT, mode permanent, scope selected, deadline NULL; generation 1 APPLIED.
- Independent running routing matrix: 38 proofs passed.
- Explicit rollback: ROLLED_BACK, target RESTORED, generation 2, profile NULL.
- Restored independent routing matrix: 38 proofs passed.
- Reality policies, client gate and the 12-endpoint snapshot unchanged.
- Final state: no active publication; user can publish their chosen settings to all servers from the deployed form.

Automatic future enrollment is verified in isolated PostgreSQL and browser fixtures; no synthetic cloud server or unsolicited permanent fleet publication was created for acceptance. Current fleet metadata remains separate from an independent fleet-wide runtime test and does not erase prior timeout evidence. App AdMob load time and actual upstream UDP support have not been measured by this feature.

Evidence/builds/rollback assets: /root/backups/dob-residential-fleet-20261005.

## Downgrade
Roll back and verify every active permanent publication with this version before removing permanent-publish support. Migration 152 down refuses active permanent profiles. Deployment fallback similarly refuses a pre-152 binary downgrade if a user has already published a permanent profile; preserve the worker needed to finish restoration. Keep all original operational evidence and do not replay prior migrations/imports/reboots.
