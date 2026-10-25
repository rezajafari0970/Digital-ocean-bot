# System audit — 2026-10-25

This is the first verified checkpoint of an ongoing audit. Inventory and file hashing do not establish line-by-line correctness. The starting repository contained 6,476 tracked files and 1,217 Go files. The audit inventory preserves read versus pending scope; historical generated memory is not proof of current semantic coverage.

## Current acceptance

| Area | Verified result |
| --- | --- |
| Source | cf7b124566df309c82c9b8a2103c848c846d9b1c; PR #4 merged as 8943b5553a44caf22d1c6f348f45b912b0a7acc7 |
| Public main/stable | 8d838efa34acc2bd6004045ccf335cc22cb5b2ef; atomic promotion run 40166549890 |
| Full source CI | 40166274739 passed: full Go tests/vet, browser checks and 189 required PostgreSQL/native/race tests with no required skips |
| Additional regressions | 25 bootstrap/backup/cluster tests, 11 updater tests, eight bounded-review tests; account browser repeated three times across six providers/four viewports |
| Public installation CI | 40166304091 passed exact Ubuntu 24.04 installation and repeated installation with identity preservation |
| Publication checks | Exact 1,723-file export manifest, both live-secret scans and gitleaks passed |
| Default instance | New public revision deployed; three actual executable hashes, five readiness checks and all 14 preservation checks passed |
| Production instance | Prior 5dc2c256 remains active and healthy; staged candidate verified; disk reserve blocked before service shutdown |
| OpenAI API | 0 requests / 0 tokens / USD 0; bounded Astra helper ready, key unavailable |

Machine-readable results: SYSTEM_AUDIT_20261025_ACCEPTANCE.json. Operational evidence is retained at /root/dob-audit-20261025. All six services had zero automatic restarts at the final observation. No provider resources, account credentials, saved scheduling/creation/output policies or fleet ownership rules were changed by this audit.

## Reproduced defects and corrections

- Database inspection accepted disabled checksums and did not recheck write-safety settings after amcheck. Settings are now validated on both sides of inspection.
- Readiness ignored the recovery-required latch and accepted undated/stale successful backup receipts. Recovery alarms, checksum failures and expired receipts now remain visible.
- Daily backup reuse mixed generation selection with mutable directory mtimes. Reuse now requires the same verified sealed generation and its timestamp; directory mtime cannot renew it.
- A delayed native dialog-close event could remove the next account form's mobile layout. Cleanup uses synchronous beforetoggle where available and ignores stale close events while the dialog is open. The regression injects stale close explicitly and also checks real-close cleanup.
- Fresh installs now use an independently identified, checksum-enabled PostgreSQL cluster. Existing database identities are preserved; no in-place checksum rewrite or adoption of unrelated data occurs.

The initial new Python fault cases failed against the baseline (seven failures and one error); Go readiness fault cases also failed. Initial integrity-only readiness is still distinct from proof of a completed backup. No corruption page was repaired or overwritten in this audit.

## Failed candidates retained as evidence

Candidate ba5a2add failed installation because PostgreSQL 16 distribution defaults lacked checksums. Candidate 42324d72 failed new cluster startup: a disposable reproduction (40165762864) proved invalid unquoted PostgreSQL configuration. Candidate a9eaef70 showed that manually added quotes were escaped again (40166175757). Reading PgCommon.quote_conf_value identified its numeric-IP heuristic; the final explicit IPv4/IPv6 loopback list is correctly quoted by the system utility and its effective value is verified.

Source run 40165891380 exposed the real dialog lifecycle defect; it was repaired and tested, not waived as flaky. None of these failed public candidates was promoted.

## Production disk block and safe continuation

The production build and fresh logical backup/encrypted archive verification completed. The updater then raised insufficient_retention_workspace before writing deployment.pending or stopping runtime services. After completed-build cache cleanup, free space was 5,965,795,328 bytes against a 6,432,375,869-byte requirement; values vary with live writes. Production still runs 5dc2c256 with matching executable hashes, five successful readiness checks and 12 unchanged policy/identity checks.

The staged release /opt/.dob-release.bXqgHMgu and its record under /var/lib/dob-production-installer are retained. ops/release/resume-system-audit-production-20261025.py defaults to inspection; --deploy rechecks the exact prior/candidate identity and invokes the normal updater only with sufficient current reserve. Run deployment through ops/release/run-bounded-update.sh. Restore disk headroom first; additional backup/archive work also needs space. Do not restart the quarantined original database, remove retained database evidence, lower reserve thresholds or restore over newer production data.

Only reproducible task exports and completed Go build/download caches were cleaned. Normal verified retention ran within the installers/backups. The final archive check is local; a fresh off-host copy is not claimed.

## Remaining audit work

| Stage | Status / next acceptance |
| --- | --- |
| Baseline, inventory and safe worktree | Complete; semantic coverage remains explicitly partial |
| Database reporting and backup freshness | Implemented and independently verified; production rollout waits for space |
| Bounded model review | Helper and tests complete; real API review blocked by missing key |
| Provider drivers and account isolation | Current-source contract review and failure fixtures pending |
| Worker retries, cancellation and ownership | Further callsite review and fault tests pending |
| Queue fairness, pool and memory limits | Measurements and bounded load/soak pending |
| Installer/restore/service failure drills | Fresh install/reinstall verified; broader isolated interruption and restore drills pending |
| Output/account boundaries and lifecycle/CDN proofs | Broader current-source review pending |
| Documentation and continuity | This checkpoint recorded in PROJECT_STATE.md, HANDOFF.md and the audit evidence |

Non-test source in internal/supervision, internal/resilience, internal/resources and internal/runtimecap was inspected. Supervision includes stage deadlines, persisted restart delay and systemd notification; that observation is not a soak-test result. Legacy Retrier/Circuit helpers have possible cancellation/half-open edge cases and no current production callsite was found, so no speculative operational change was made.

Original-host physical storage attribution, invalid AWS credentials, selected DNS capacity and offline gateway management remain external constraints. There is no blanket zero-error, automatic recovery from every future failure, or permanent uptime guarantee. Recovery must be bounded, preserve ownership/data and verify its outcome.
