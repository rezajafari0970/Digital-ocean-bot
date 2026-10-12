# Video follow-up — 2026-10-12

**IN PROGRESS — BLOCKED FINAL API REVIEW. No new public release or production deployment.**

Candidate branch: fix/video-followup-20261012. Base/canonical: a1f118d5351b24e6946a5acf0176cf738c9a7ee5. Both hosts still run 5b0dc7640eaf51909ae04a1710aae15bf34088ab. Independent final review is mandatory under the user's accepted workflow; it is not waived by local test success.

The complete218.546-second video was inspected at1-second coverage with additional readable sheets. Fixes cover canonical provider inventory counters, missing-versus-empty inventory, cadence-consistent freshness metadata, removal of automatic refresh POST on account viewing, recovered create errors, current-step history counts, concurrent provider auth diagnostics, runtime version separate from historical update receipt, explicit eligible-scope recovery wording and duplicate-click guard, historical tuning labels, and loading/error/empty/mobile rendering of the opt-in human Output view. Raw share formats and class/filter boundaries remain unchanged. Orchestrator CLI requires an explicit job; help/invalid input cannot start API work.

Tests:59Go packages with isolated PostgreSQL and optional browsers PASS; affected adminapi package rerun PASS; vet PASS; Chromium26scenarios PASS; output URI3tests PASS; CLI2tests PASS; final raw/opt-in output contracts PASS. Earlier browser harness races and missing synthetic user gesture were corrected; initial failure logs remain preserved.

Initial independent review returned REVISE for freshness threshold metadata and missing inventory wording. Both have code and regression evidence. Final review requests did not return a verdict; the last rejection is HTTP429 insufficient_quota/credit_balance_exhausted. Do not call the source independently approved.

API this phase:3 intentional submitted requests,1 completed response,2 failures. Confirmed receipt37002input+824output=37826tokens. Legacy accidental --help invocation may have made additional unreceipted calls; count/tokens unknown. Prior two phases had232325confirmed tokens; known receipt aggregate270151tokens across12responses is not total account usage. Dollar charge unavailable. No further API calls are authorized by this script.

The accidental legacy diagnostic invoked the old default PLAN job, stopped before implementation, preserved old job history and recorded INTERRUPTED. No production code changed. Its unreceipted API usage is never reported as zero.

Live read2026-10-12T20:19:08Z:primary57tracked=45active+5degraded+7pending deletion;0active/waiting builds. Each host has3active services and5/5readiness checks. Linode9READY after natural expiry; a new replacement107991915 reached PANEL_COMPLETE at20:10UTC, a later expiry107979556 was natively deleted20:15:59UTC, next configured build20:25:14UTC. Do not accelerate20–30minute spacing. Client capacity last-observed215target/211active/4deficit,35of43observations fresh,8stale,0missing; not a simultaneous live total. Vultr4335 actual401/TOKEN_INVALID remains external.

Evidence and exact file hashes: /root/backups/dob-video-followup-20261012. Machine-readable findings and resume instructions: docs/development-jobs/video-followup-20261012.json. Resume at independent final review once credit is restored; preserve public install/scanning gates and both-host acceptance. Do not advance canonical or publish from this blocked checkpoint.
