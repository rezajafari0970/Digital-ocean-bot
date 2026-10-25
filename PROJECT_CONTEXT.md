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

## Configuration cleanup and TCP HTTP release — 2026-10-25T17:34:22.028560+00:00

Read docs/CONFIG_CLEANUP_HTTP_20261025.md and its acceptance JSON first.
Product bf56dd5c311aea8393d316674fa9e5113cfcf2e6 merged via PR12 as
9428375577cb36403a5f9da6e7d8ad39361287e8. Canonical branch
checkpoint/final-e2e-20260929; worktree /root/projects/dob-ssh-durability-20261025.
This documentation/operations checkpoint is a child of that merge.

Public main/stable and BOTH live instances now run
64fd97ed3d25198e7400b4f5701477e94cc92896, schema196.
Source CI40214540128 passed 220 mandatory roots/0 skips, native/race/browser,
20 isolation repeats and disposable restore196. Public CI40214736483 passed
fresh/repeated install. Atomic promotion40215046466 passed and refs verified.
Both bounded updates passed all15 preservation checks,3 binary hashes,5 readiness.
Default has no profile identities; production baseline6,179 profiles/77 Trojan.
See JSON for exact final times and allowed lifecycle changes.

Video reviewed: 33.44s; sampled remaining73/79/66;79->77. Current low-space
database attention was resolved by moving7 old routine archives (5,211,867,925 bytes)
to original host, destination+source SHA verified before unlink. Incident/current
backups and unrelated data retained. Private receipts under
/root/dob-config-cleanup-http-20261025; original archive receipt directory
/root/dob-config-cleanup-archive-20261025. Never discard these as disposable.
Final JSON records disk/ready; permanent storage growth still needs capacity.

Creation workers now assist confirmed deletions; deletion/proof priority preserves
proof deadlines; independent tombstone completion and late-write fencing fixed.
TCP HTTP Header supported via shared Spec/driver/API/UI/export, Host default
play.googleapis.com, path/. Protocol-specific URI formats retained.
No Android device acceptance or measured after-change fleet speedup claim.

Three pending deletes remain on one expired AWS server whose account TOKEN_INVALID
blocks provider confirmation and whose panel is unavailable. Keep durable intent;
no force retirement/credential guessing. Correct through Accounts UI, then observe.
Whole-project audit remains INCOMPLETE:235 full current files/27,909 lines;
188/1,218 baseline Go files/22,340 baseline lines. See explicit partial-read inventory.
Next audit: application/provider ownership and resources; reproduce missing rows.Err
check in application confirmDeletedTx. No error-free or permanent-uptime claim.

Original project cluster18/main remains down; app workers/publisher quarantined.
Do not restart/heavy-test original host. Its postgresql umbrella unit is exited.
Original API ledger still1 unknown request,USD1.501011 reserved,USD2/3attempt cap;
zero new paid generation. Resolve usage/result before another paid review.
No ledger reset, duplicate unknown operation or old8d838efa stage deployment.
