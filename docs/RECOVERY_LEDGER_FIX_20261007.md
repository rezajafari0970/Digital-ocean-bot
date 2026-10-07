# Durable recovery ledger correction — 2026-10-07

Runtime/API/worker revision50f9ffb6ac20012c0e57e068fbde2aef0905833b deployed at2026-10-07T09:16:24.915929+00:00. Migration162 applied through the normal runner; static0c7721d and all checked account/profile/gate/route/proxy/protection settings preserved. Final verification at2026-10-07T09:17:13.931255+00:00.

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

## Production evidence
- Exact committed worker artifact passed separate-systemd role restart/peer survival/duplicate-role refusal in an empty isolated PostgreSQL schema.
- First deployment attempt correctly refused a stale prior-process heartbeat; zero-pending checkpoint gate permitted actual production rollback. The deployment wait was corrected to require BOTH current process IDs, then the same reviewed binaries deployed successfully. Original attempt evidence is retained.
- API, control and panels processes remain active with matching build hashes and no automatic restarts; all five readiness checks pass. Current-process logs contain no new global gate closure, panic, ownership loss, completion or reconciliation error.
- Fresh native panel audit matched304 published configurations across33 panels; all304 existed, were enabled and unexpired; no unmatched, invalid or unavailable results. Output later sampled35 Direct/280 Residential entries as native lifecycle continued.
- By09:16:42UTC,20 create and19 delete jobs created since rollout had succeeded. Twenty previously-created owned clients expired/deleted after rollout after at least595 seconds; same-scope replacements were observed. This is not a claim of a full post-rollout-born10-minute cohort.
- Pending durable checkpoints decreased from3 to1 as work completed; the final remaining deployment checkpoint was9.7seconds old and in-flight, not a stuck orphan. Both UpCloud113 live servers were READY; Desired5 unchanged.
- Read docs/RECOVERY_LEDGER_FIX_ACCEPTANCE_20261007.json for timestamps, hashes, full review findings/limits and native evidence.
