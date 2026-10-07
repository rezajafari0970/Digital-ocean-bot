# Durable recovery ledger correction — 2026-10-07

Candidate tested and independently reviewed; deployment evidence follows after rollout.

## Defects and corrections
- PH12-001: recovery discovery now propagates ledger/pruning errors before any installer bypass or native handler.
- PH12-002: BEFORE-handler durable checkpoints and per-item session ownership fence recovery admission. Failure/backoff or successful clear commits atomically with checkpoint removal. Failed or uncertain completion leaves its durable checkpoint.
- Reconciliation never invokes a native handler. Inactive orphaned attempts receive a persisted30-second interrupted-outcome delay that installer bypass cannot override; ordinary native fences still govern any later retry.
- Independent review found the equivalent lifecycle completion gap. Lifecycle now uses the same runner, its own filtered reaper, and checked progress.
- Failed active keys remain latched in readiness until a later inactive reconciliation scan succeeds. Older successful scans cannot hide newer async failures.
- RecoverOperation's deferred diagnostic SQL now shares a bounded5-second cleanup context. Original errors are preserved.
- Checkpoint sessions are inside shared control3 admission. The modeled conservative per-job SQL allowance is6; control24/panels40/API24 caps are unchanged.

## Validation
See paired ACCEPTANCE and SOURCE_SNAPSHOT files. Actual OpenAI API review required three rounds; findings were corrected before final PASS. The final fixture-only correction reuses its pre-created accounts table and does not change product code; the real locked-table test passes afterward. Original failed logs are retained. PostgreSQL fault triggers, race tests, phase1/phase2 tests, HTTP uncertain-outcome regressions, full Go and rollback launcher checks pass. No production faults were injected.

## Deployment and rollback contract
Migration162 is additive. Build from a clean committed freeze; test that exact worker artifact in separate systemd roles with an empty isolated database. Preserve existing service topology, static assets, gates, account/profile/route/proxy settings. Stop both workers before shared-binary replacement. Let the normal application migration runner apply162. Verify actual running hashes, two role owners, pools and all five readiness checks.

Use tools/recovery-ledger-rollback.py with the saved backup directory and authorized environment. The entrypoint stops BOTH workers, then refuses any old-code restoration if checkpoints remain or the database cannot prove zero. It leaves schema162 in place. Never bypass this gate by manually starting checkpoint-unaware binaries. The SQL down migration separately refuses unresolved checkpoints.

## Limits
This is conservative recovery, not exactly-once remote side effects or universal automatic repair. Provider billing/trial limits, explicit policy gates, quarantine, ownership and finite budgets remain authoritative. Host/database/network isolation, watchdog coverage for arbitrary stuck native handlers and wider queue fairness remain separate work. Lost COMMIT acknowledgement and heavily saturated100-row reaper scenarios lack direct injected coverage; native production observations are bounded evidence.
