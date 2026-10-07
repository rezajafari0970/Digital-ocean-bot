# Phase1/2/3 remediation — 2026-10-07

Status: deployed and bounded native verification passed.

Baseline f4dadf1754122081d7bc850bcb0f69427c7312e2. Ten original findings and supplementary review defects corrected. Independent actual server OpenAI API response resp_029a4be631125ff6006ac640e5fdb487d1a7294bff85abb394 returned PASS with no open findings.

| Finding | Area | Correction |
|---|---|---|
| PH123-001 | Checkpoint/account isolation | Removed-account reconciliation, purge fence, per-row250ms/round2s bounds, retained keyset cursor under completion+diagnostic faults. |
| PH123-002 | Fresh installation | Always install/enable both roles and API/control drop-ins; validate effective topology. |
| PH123-003 | Capacity liveness | Bounded batches admit before runtime acquisition and track each native task. |
| PH123-004 | Upgrade convergence | Shared unit topology install/reload, matching rollback assets, unresolved-work rollback refusal and preflight override conflict detection. |
| PH123-005 | Scanner liveness | Independent async scan deadlines; child progress cannot extend scan lifetime. |
| PH123-006 | Discovery fairness | Due filters precede limits; keyset deferred bypass cursor, same-round deduplication and oldest-per-account retirement rule. |
| PH123-007 | Residential failure handling | Checked outcome, durable preclaim, cancellable pacing and circular panel selection. |
| PH123-008 | UTF8 persistence | Normalize invalid UTF8/NUL and truncate to2048 bytes on rune boundary. |
| PH123-009 | Workflow persistence | Checked completion, generation-scoped durable successful results, atomic admission/attempt/start event and all terminal state/resource/event writes. |
| PH123-010 | Recovery terminal branches | Terminal deployment/resource/event transaction and checked installer failure persistence. |

Validation: focused PostgreSQL/race cases, retained phase1+2 PG/HTTP/native fault tests and full Go suite, phase3 supervisor tests, real isolated systemd SIGSTOP/watchdog/peer-isolation fixture, and sixteen complete install/upgrade shell-model scenarios passed. Fault injection was confined to a separate test database and disposable service units.

Additional review-driven corrections: independent OS role ownership survives SQL connection loss; locked/poison checkpoint rotation; terminal and intermediate workflow event atomicity; lost completion acknowledgement recovery; native retry budget is charged atomically with admission; unsupported conflicting operator drop-ins fail before runtime changes.

Rollback: stop both roles and API; refuse the old binary if any recovery checkpoint or successful workflow result awaiting application exists or safety cannot be read. Restore matching binaries and managed units together; keep additive schema163. Preserve all operator configuration. Never delete an unresolved checkpoint to force rollback.

Limits:
- No guarantee of permanent absence of faults; external provider, PostgreSQL, host, disk and network failures remain possible.
- Host role fence assumes the supported single worker host and shared canonical state directory; multi-host active/active fencing is not implemented.
- Unresolved item persistence stays fenced and exposes degraded readiness; healthy accounts continue, and readiness alone does not restart a healthy scanner.
- Operator pauses, provider/trial restrictions, quarantine, ownership/unknown outcomes and finite budgets remain authoritative.
- Install/upgrade scenarios execute complete shell scripts with mocked OS commands, not disposable real OS installations. The real systemd fixture separately validates the exact worker process contract.
- Release rollback retains initialized database, credentials and data intentionally for safe retry; it restores the complete managed runtime artifact set, not database bootstrap side effects or whole-host power-loss atomicity.
- Native readback checks configuration identity/enabled/expiry; it does not prove mobile traffic or the complete residential data plane.

## Final runtime proof
Revision dacc3329b5e2969415fa7051e34356e39d549c61; schema163; frontend unchanged. 49 healthy samples over 481.1s, no automatic restarts. Native 10 configs/4 panels verified. 6 new successful config create jobs; 0 delete jobs observed. No new expiry-cycle claim under existing lifetime0 settings. Five Vultr TOKEN_INVALID and one disabled DO BILLING_BLOCKED remain separate operational blockers. No production fault injection or rollback. See PHASE123_FINAL_REAUDIT_20261007.md/JSON for final limits.
