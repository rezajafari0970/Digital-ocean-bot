# Worker isolation phase2 — 2026-10-07

## Status
DRAFT_IMPLEMENTED_TESTED_NOT_DEPLOYED. Production is still API/worker caf3f0de76d142b7e8ded2923a429b00fd4ba50c, static0c7721d/schema161. The server OpenAI development adapter returned HTTP429; a minimal diagnostic identified credit_balance_exhausted / insufficient_quota. Its plan and independent source review are not complete. Do not mark that gate PASS, automatically retry exhausted credit, or install this draft yet.

## Phase1 production acceptance
The user resumed the real gate at06:23:13Z. At06:37Z,832 new BULK_CREATE and166 BULK_DELETE jobs were SUCCEEDED;159 newly created owned clients had actually lived at least595 seconds and then been deleted. Same-panel/inbound replacement pairs were observed. No postdeployment global fail-close, panic or ambiguous next_retry_at error was found. Actual06:40Z public class shares were Direct58 and Residential446, both HTTP200. Counts vary with expiry and fleet lifecycle.
Both UpCloud replacements are READY/PANEL_COMPLETE;16 fresh visible snapshots were present at06:37Z. Desired5 is preserved and provider capacity remains separately constrained. This is native control-plane/output evidence, not mobile traffic or indefinite availability proof. No further Resume is required for the observed state.

## Implemented draft boundaries
- Explicit module registry with all/control/panels roles. Unknown roles and duplicate registrations fail startup. All panel-native work retains one shared Sanaei runtime manager in the panels process; cloud lifecycle/recovery/scheduler and account/network control move together to control. Default all preserves existing deployment topology.
- Module membership is tested against the real main wiring. Unexpected module return or panic fails that role process. There is no automatic in-process replay of possibly committed work. Normal cancellation joins modules; service termination has a90-second limit.
- PostgreSQL session ownership rejects duplicate roles and mixed all+split. One pinned session per process monitors both backend identity and exact role advisory locks. Connection/lock loss ends the role process. Existing durable operation/account/client leases still govern remote mutations; role ownership does not replace them.
- Split DB caps are control24 and panels40, API24, preserving the old88 total. Heartbeats expose pool statistics. Each role has independent512MiB MemoryMax,256MiB MemoryHigh,256 tasks and8192 descriptor settings in its service configuration.
- Explicit API DOB_WORKER_MODE=split requires fresh production-control and production-panels records. No legacy-worker fallback can conceal a missing role. Missing, malformed, stale and future progress fails readiness; policy GATED remains distinct from successful mutation work.
- Install/upgrade preserve an already-selected split topology and stop/restart its panel service during binary replacement. New ordinary installations remain all-role unless split is explicitly configured.
- The earlier prepared-readiness PG fixture now includes the lifecycle/client progress contract introduced in phase1.

## Verification
tools/worker-isolation-acceptance.sh passed: real isolated PostgreSQL ownership/session-loss/readiness tests, OS-process separation, normal shutdown join, role membership, targeted race tests, native HTTP response-loss/no-duplicate regression, ownership/capacity/expiry tests and full Go suite.
tools/worker-isolation-process-fixture.py uses a random isolated schema in bulk_test, a newly generated master key, empty account inventory and loopback-only transient services. Real worker binary role startup passed. Killing only the fixture panels MainPID produced one automatic systemd restart, a new panels PID/heartbeat, unchanged control PID and duplicate-role rejection. Test services were stopped; schema and fixture secret files were removed. Production services, gates and settings were not changed.
The first process-fixture attempt failed because psql does not accept the Go driver's search_path URL parameter. The SQL observer now maps it to PGOPTIONS. Original failed result/logs remain alongside the successful run. This was a test observer defect, not a product heartbeat failure.

## Required continuation
1. Restore the existing OpenAI API credit, then resume the Development Orchestrator job worker-isolation-20261007 without forging completed phases. No re-priming.
2. Run the prepared source review /root/backups/dob-worker-isolation-20261007/review.py with the authorized OpenAI environment; inspect/adjudicate actual findings and regenerate the source snapshot after any changes. verify-review.py requires a genuine PASS and successful test evidence.
3. Review loaded DB lease headroom before production promotion: an empty-fixture startup does not prove busy-fleet throughput under the24/40 split. Bound outstanding leases or adjust their partition within the authorized total if evidence requires it. Do not merely raise PostgreSQL capacity or weaken guards.
4. Build from a clean reviewed checkpoint; back up binaries, manifest, exact current service/drop-in topology and configuration. Stop the old all-role worker before starting control/panels. Install deploy/worker-control.conf into the worker service drop-ins, deploy/api-split-workers.conf into the API drop-ins and the panels unit. Verify both role leases, running hashes, readiness, pool waits and actual generation/expiry output.
5. Rollback must first stop both split workers, restore previous binaries and exact previous topology/drop-ins, reload systemd and restart that topology. Preserve gates, profiles, finite budgets, quarantine, ownership, native uncertainty and all current settings. Never start an old all-role binary alongside split processes.

## Remaining scope
No complete host/database independence; split roles still share PostgreSQL, host and network. No watchdog for every living-but-stuck module, queue-wide fairness, independent proxy evidence or arbitrary-bug repair. Global client concurrency1 remains. Crash/restart correctness for every remote operation still relies on existing durable reconciliation; the phase2 process fixture does not create real cloud/panel mutations.
Evidence: /root/backups/dob-worker-isolation-20261007. See paired ACCEPTANCE and SOURCE_SNAPSHOT JSON before continuing.
