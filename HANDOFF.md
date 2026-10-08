# Latest continuation — residential allowlist, 2026-10-08

Read `docs/RESIDENTIAL_ALLOWLIST_ACCEPTANCE_20261008.json` and `docs/RESIDENTIAL_ALLOWLIST_20261008.md` first. Runtime `7cc709e` has strict residential allowlist active fleet-wide; independent serving-panel proof22/22 and installed-client canary9/9 passed. This does not include73 expired/retiring/deleting panels or mobile compatibility.

**Unfinished:** four distant panels get false periodic FAILED/APPLYING from the old8second whole-proof timeout. Tested fix `3fad477` on branch `fix/residential-proof-timeout-20261008` is not deployed. Independent API review is blocked by exhausted credit(HTTP429 `credit_balance_exhausted`). Do not bypass the review gate or silently claim the entire rollout is complete. Restore review availability or obtain independent human review, integrate latest canonical, deploy guarded, verify periodic APPLIED state.

**User reporting preference:** give meaningful progress updates at least every60seconds during active work; persist timestamp/current action/result/blocker/next action; distinguish WAITING_EXTERNAL/BLOCKED from active agent execution and from normal application workers.

The sections below are historical. Preserve their evidence but verify live source/runtime before acting.

---

# Current production continuation — 2026-10-03

The current frontier is durable owned-user lifecycle, accepted and enabled on eligible v3 panels. Start with `docs/BULK_LIFECYCLE_ACCEPTANCE.json`, `docs/PRODUCTION_REVISION_STATUS.json` and the final lifecycle section of `docs/DURABLE_BULK_V3_HANDOFF.md`. Verify live source/DB/runtime first.

Runtime source b789a47639f5299e84093024262d799edaf644fb; canonical branch checkpoint/final-e2e-20260929. Twelve lifecycle scopes were active at acceptance. Main execution gate is deliberately open for bounded continuous operation, concurrency=1; old bulk canary gate remains closed. Two historical CREATE failures are excluded and preserved.

The four requested parts—policy updates, owned cleanup, replacement/Output and gradual production rollout—are accepted. Do not replay the earlier 10000 stage or the archived console task below as the current next step. Retain all failure audit and read before retrying mutations.

## Archived console checkpoint

# Handoff
Updated: 2026-10-01

## Resume here
Current workstream: Vultr interactive console access through reverse proxy/noVNC.
Latest: 6834d81 — accept root noVNC websocket alias.
Previous: 7126567 websocket path; af598d7 short session cookie; 7c9f335 interactive challenge session; 7b79dad console capacity browser probe.

## Exact next action
1. Refresh/re-login Admin so obsolete pre-restart session is not reused.
2. Click Open Vultr Console for a fresh ticket/session.
3. Verify noVNC leaves connecting state and WebSocket upgrade succeeds.
4. Solve Cloudflare/CAPTCHA/2FA manually if presented.
5. Verify the authorized Vultr session can be reused by capacity/account workflow.

## Working tree warning
Do not delete/reset .audit blindly. At checkpoint creation source matched origin at 6834d81; .audit had modified/untracked analysis artifacts. Treat them as audit evidence, not source changes.

## Knowledge handoff status
- Fresh-session acceptance gate: 100/100 READY at the latest generated knowledge snapshot.
- Handoff readiness audit: 38/38 PASS.
- Primary new-chat entrypoint: `docs/NEW_CHAT_ENTRYPOINT.md`.
- Unified retrieval: `tools/project-brain-query.py`.
- Exact source memory: `docs/FULL_SOURCE_KNOWLEDGE.json` + `tools/source-lookup.py`.
- The new session must verify drift/current HEAD before treating snapshot line numbers as current.
