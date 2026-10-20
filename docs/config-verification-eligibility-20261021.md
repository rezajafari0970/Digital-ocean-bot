# Configuration verification admission — 2026-10-21

At 02:34 Tehran both controllers remained healthy on public revision `2a9d64b`; all services had zero automatic restarts since installation. The Cloudflare recovery had converged: 61 eligible servers had 122/122 published class outputs, all current managed DNS records had no provider error, and the former PREPARED queue was empty. The selected zone still reported a 200-record quota, with observed usage 198; admission remains constrained by that provider quota.

A separate scheduling defect remained: one enabled, unexpired server belonged to an account marked TOKEN_INVALID. Its four listeners retained stale verification timestamps and were repeatedly claimed every ten seconds, although downstream publication and native access correctly rejected the account. Ordinary and recovery workers could waste attempts on this ineligible target.

The verification claim now uses the same shared EligibleTarget predicate as downstream verification and publication, before taking a lease or opening a panel session. Creation, deletion and uncertain mutation reconciliation retain their existing independent paths. Eligibility is evaluated from current database state on each claim, so restoring an account automatically allows its due work to resume. Credentials and stale evidence are never rewritten merely to make a status appear healthy.

A PostgreSQL regression reproduces the defect before the fix for invalid-token, locked, disabled and deleting accounts, disabled panels and expired servers. For each case it exercises routine/recovery consumers and both the callback and production batch paths, requiring unchanged listener evidence and scheduling. The fix passes all cases and verifies automatic recovery after admission is restored. Focused race tests also cover lifecycle resize/delete/recovery, unfinished cleanup exclusion and legacy retirement.

Evidence: `/root/projects/dob-proof-eligibility-20261021/.local/before.log` reproduces the failure; `after.log` passes. OpenAI API usage: zero. Release validation and sequential production rollout are pending.
