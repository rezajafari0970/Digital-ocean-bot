# Operational repair release accepted — 2026-10-09 Tehran

Runtime/product source **897f40800d047a3f08faccb501e450a17208b6bc** is published and independently verified. Read `docs/OPERATIONAL_REPAIR_STATUS_FA_20261009.md` and `docs/OPERATIONAL_REPAIR_ACCEPTANCE_20261009.json` first, then the detailed delta. This checkpoint updates documentation only; do not redeploy unchanged binaries because canonical documentation is newer.

Completed: deletion UI/error handling and safe operational/profile purge; guardian packaging and idempotent service reconcile; native provider warning detail; bounded idle provider polling. Actual API product PASS: `resp_0af18c241cde442e006ac8215607fc87d1a505ce711fa1a94b`. Full Go/race, browser, installed-core and packaging evidence binds to source digest `220584a92ade3f3d3bfe178f9250526b4a71f706a739a7c957d2e72810907f97`.

Remaining: **90% reduction not verified**, invalid-token cloud cleanup and transient resource pressure. Current wire rates are 235,751 → 224,044 bytes/minute, but cohort/workload changed; do not claim causation or comparable 90%. Existing two-minute dashboard freshness may conservatively show an idle five-minute observation as unverified. Do not hide alerts or weaken freshness/protection.

Evidence: `/root/backups/dob-operational-release-20261009`; original deploy EXIT marker is absent, independent `deploy-verification.exit=0` and `publication-verification.json` verify publication. Documentation workflow: `/root/backups/dob-operational-handoff-20261009`. Live continuation: `.local/account-panel-repair-progress.json`. During active work report timestamp, concrete result, blocker and next action at least every 60 seconds. Completion of this bounded release does not mean every operational goal is resolved or that an AI agent continues unattended.

The residential section below is preserved as a timestamped historical acceptance; its product changes remain in this release.

---

# Latest continuation — residential allowlist, 2026-10-09 Tehran

Read `docs/RESIDENTIAL_ALLOWLIST_ACCEPTANCE_20261008.json`, `docs/RESIDENTIAL_ALLOWLIST_20261008.md` and `docs/RESIDENTIAL_ALLOWLIST_STATUS_FA_20261009.md` first. Reviewed verifier fix **d02f045** is deployed; API credit blocker is resolved. Runtime hashes are verified and all five readiness checks pass. At2026-10-08T22:39:27Z current serving native proof and periodic state both pass **30/30**. Canonical report commit is newer than the runtime source because it changes documentation only.

**Remaining operational work:** at that snapshot4/30 serving panels had CPU-pressure/recovery admission blocks. Both client versions work; on the loaded canary TCP443 refusal aligns with existing guardian protection, not a demonstrated version-specific TLS defect. Pinned actual traffic is26/27 aggregate: target26.9.9 and installed-repeat26.3.27 each9/9. Preserve the initial transient failed probe; do not claim27/27 or mobile acceptance.74 expired/retiring/deleting records and1631 historical DELETED records are excluded from serving proof.

**Next phase:** capacity/connection stability and mobile local-DNS/domain handling; design safe capacity-aware output/failover before changing product behavior. Preserve guardian protection, quotas/lifetimes, spend, mutation gates and independent review. The agent ends after this report; normal application workers continue. No unattended AI task is implied.

**User reporting preference:** meaningful progress updates at least every60seconds during active work; persist timestamp/current action/result/blocker/next action; distinguish IN_PROGRESS/WAITING_EXTERNAL/BLOCKED/COMPLETED from normal application workers. Live continuation status: `.local/residential-allowlist-progress.json`.

The sections below are historical. Preserve evidence but verify live source/runtime before acting.

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
