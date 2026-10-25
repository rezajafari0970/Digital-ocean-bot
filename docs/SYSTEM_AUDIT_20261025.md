# System audit — 2026-10-25

This is an in-progress audit, not a completed line-by-line review. The machine-readable inventory records source hashes, module size, and explicitly read versus pending files. The baseline has 6,476 tracked files and 1,217 Go files. Historical generated memory is not evidence of current semantic coverage.

## Verified starting point
The reachable recovery host has two independent installations of public revision 5dc2c25678698dbbe9ac1cfee7d18701c19213e7. All six services and both five-check readiness reports passed. The original host remains quarantined and offline. The source checkpoint is e13e150d3026b2586e491dd51efdd072c22c426a. No production policy or credentials were changed. Free space is constrained; retain deployment reserve gates.

## First reproduced defects and correction
- Inspection accepted disabled page checksums and did not recheck write-safety settings after amcheck.
- Readiness ignored the recovery-required latch and accepted undated or stale successful backup receipts.
- Daily backup reuse combined generation selection with mutable directory mtimes.
The corrections fail closed, check settings on both sides of the scan, retain recovery alarms and bind freshness to the same verified sealed generation. Initial integrity-only readiness remains distinct from proof of a completed backup.
The new Python fault cases failed against the baseline (7 failures and 1 error); Go readiness fault cases also failed. After correction, 25 bootstrap/owned-cluster/backup tests, 11 updater tests and the observability package pass. The first exact-source full suite, browser checks and 189-test race gate passed (run 40164527318). Public candidate ba5a2add failed fresh installation on PostgreSQL 16 because the distribution default lacked checksums; that candidate was not promoted. Fresh installs now create a separately owned checksum-enabled cluster and real install/reinstall must be rerun. Deployment has not yet occurred.

## Work sequence and acceptance
1. Baseline and source inventory: complete; not a complete semantic review.
2. Database reporting/backup freshness: implemented; independent validation pending.
3. API review and consumption guard: bounded Astra helper implemented with eight fake-transport tests; key unavailable in both known instance configurations and current process; 0 requests/tokens/USD this turn. Do not invoke the existing multi-role council without an enforced aggregate budget. The existing adapter has no durable aggregate cost reservation.
4. Worker/account/provider isolation, retries and ownership: pending current-source review.
5. Queue fairness, timeout/cancellation, pool and memory limits: pending measurements and fault tests.
6. Provider driver contracts and installer recovery: pending per-provider contract review.
7. Output/account separation, expiry and CDN proofs: pending current-source review.
8. Capacity/load soak, database restore drills and service-failure drills: pending independent fixtures; no destructive production experiments.
9. Exact-source CI, clean public export and rollback-aware rollout: pending for this patch.
10. Final evidence, open risks and continuity: updated at each checkpoint.

## Constraints and unresolved risks
No blanket no-downtime or future self-repair guarantee. Recovery must use bounded retries, persisted ownership and verified outcomes; never erase a corruption alarm or restore over newer production data automatically. Physical storage failure attribution is unresolved. AWS credentials, selected DNS capacity and offline gateway management are external blockers, not reasons to weaken health checks. Heavy race/native/load testing belongs on isolated runners, not this 4 GiB recovery host.


## Further source inspection
The non-test code in internal/supervision, internal/resilience, internal/resources and internal/runtimecap was read. Supervision has per-stage deadlines, a persisted restart delay and independent systemd notification. This observation is not a soak/fault-test result. The legacy Retrier/Circuit helpers have possible cancellation/half-open edge cases; no current production callsite was found in the source search, so no operational fix or performance claim is made. These remain follow-up contract-review items.
