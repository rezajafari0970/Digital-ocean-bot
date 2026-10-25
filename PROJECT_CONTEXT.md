# Project Context
This repository is the durable source of truth for the DigitalOcean + Vultr automation project. Chat history is not the sole project record.

## Core rules
- Keep provider integrations modular and independent.
- DigitalOcean and Vultr capacity/account logic uses provider-specific authoritative data.
- Sanaei/x-ui live reads/writes use its API where implemented; Output must not depend on SSH reads.
- Output is live/fast rather than stale cached data.
- Proxy/account/provider modules remain independently testable.
- CAPTCHA/2FA challenges are completed interactively; authorized sessions may then be reused where appropriate.

## Continuity protocol
Before ending a development session: update PROJECT_STATE.md and HANDOFF.md with branch/HEAD, tests, deploy state, migrations, blockers, and exact next action; commit and push. Never store passwords, API tokens, private keys, cookies, or session secrets in these documents.

At a new chat/session: read PROJECT_CONTEXT.md, PROJECT_STATE.md, HANDOFF.md, then verify git status/HEAD and production service state before changing code.

## Recovery, retention and runtime memory — 2026-10-25T18:50:05.118451+00:00

Read docs/RECOVERY_RETENTION_20261025.md and its acceptance JSON first.
Product a400e455a67bc95fa51cbafb77b22b40508c66c5 merged through PR13 as
11b390020a9e611f564167c0e04fd76552b45475. Canonical branch checkpoint/final-e2e-20260929;
worktree /root/projects/dob-ssh-durability-20261025. This documentation checkpoint
is a child of that merge; git HEAD identifies the documentation commit.

Public main/stable and BOTH live instances run 53294d3936a96fa342db4036fa87f910f289e1e9,
schema197. Source CI40220965327 passed 230 mandatory roots / zero skips, full
Go/vet/browser, native transports/race, 20 lifecycle repeats and restore197.
Public CI40221257928 passed fresh/repeated installation with identity preservation.
Atomic promotion40221609420 passed. Both bounded updates passed all 15 preservation
checks, three binary hashes and five readiness checks. Production baseline:
6183 profile identities / 77 Trojan; no unexpected changes or missing baseline
identities. Five independently confirmed expiry retirements and six normal profile
additions; all Trojan identities unchanged. API controlled stop: 216.581s.

Fixed interrupted deletion confirmation rollback and 20 further SQL iteration-error
boundaries (55 API fault cases). Retention runs round-robin, up to ten batches of
1000 per table within the original ten-second deadline. Migration197 provides the
completed-only witness index; stale-statistics plan reproduced, then fixed without
extending acceptance timeouts. Runtime mutation gates now exist only for callers/
waiters; expired cache/circuit entries are swept. Regression failures and intermediate
failed runs are preserved. This is targeted verification, not a universal safety proof.

New-worker retention observation: 1867 observations removed in
1 successful event(s), 8 eligible panel rows
at 2026-10-25T18:49:42.316682+00:00. These are internal observations, not user configs.
HTTPS version/readiness/UI hashes and normal certificate validation passed.
Current DB checksum counter zero; integrity and backup healthy. Latest free disk
7859412992 bytes. Eight superseded archives totaling 5,826,809,115 bytes
relocated with verified destination/source hashes; current and incident archives
retained. Private receipts: /root/dob-recovery-followup-20261025 (recovery) and
/root/dob-config-cleanup-archive-20261025 (original). Never treat them as scratch.
Current backup: /root/dob-recovery-followup-20261025/production-rollout/database-before.dump.

Earlier TCP HTTP Header feature and deletion-priority improvements remain installed;
see docs/CONFIG_CLEANUP_HTTP_20261025.md. Protocol-specific encoding is retained.
Three configuration deletions still await one expired AWS server: TOKEN_INVALID,
InvalidClientTokenId/GetCallerIdentity, panel and SSH TCP timeout from BOTH controllers.
Keep durable intent; do not force retirement. Restore authorized account/network
access through Accounts UI, then observe normal cleanup. No credentials in chat.

Original cluster18/main, project API/workers and publisher remain inactive (verified
18:47:32 UTC). Historical invalid-page/checksum corruption is confirmed; physical
cause is not established. Do not restart or heavy-test that quarantined host.
Use independent runners for native/race/database tests and bounded production updates.

Whole-project audit remains INCOMPLETE: 269 full current files /
31720 lines; 214 / 1218 baseline Go paths, 25264 / 146345
baseline lines. Partial reads are explicit in SYSTEM_AUDIT_RESTART_20261025.json.
Next audit: remaining application/provider ownership and resource paths, following
the module inventory; do not re-report confirmDeletedTx as unfixed.
No new paid API generation: one prior unknown request retains USD1.501011 reserve
under the existing USD2 / three-attempt cap. Resolve usage/result before another
paid review; do not reset or replay the unknown request.
No indefinite uptime, complete future self-repair, Android acceptance or measured
fleet configuration-deletion latency claim.
