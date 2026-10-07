# Worker isolation phase2 — 2026-10-07

## Status
DEPLOYED_SPLIT_NATIVE_PROGRESS_VERIFIED. Clean source 951ea1a5852e716d09f4362e75f1a4ba51ca3c7a is running as API plus control and panels worker processes. Deployment completed at 2026-10-07T08:17:09.804787+00:00; all five readiness checks passed throughout the recorded production observation ending 2026-10-07T08:18:50.474410362Z. Static0c7721d and schema161 are unchanged. No further Resume or credit recharge is required.

## Phase1 production acceptance
The user resumed the real gate at06:23:13Z. At06:37Z,832 new BULK_CREATE and166 BULK_DELETE jobs were SUCCEEDED;159 newly created owned clients had actually lived at least595 seconds and then been deleted. Same-panel/inbound replacement pairs were observed. No postdeployment global fail-close, panic or ambiguous next_retry_at error was found. Actual06:40Z public class shares were Direct58 and Residential446, both HTTP200. Counts vary with expiry and fleet lifecycle.
Both UpCloud replacements are READY/PANEL_COMPLETE;16 fresh visible snapshots were present at06:37Z. Desired5 is preserved and provider capacity remains separately constrained. This is native control-plane/output evidence, not mobile traffic or indefinite availability proof. No further Resume is required for the observed state.

## Deployed boundaries
- Explicit all/control/panels module registry. Unknown roles and duplicate registrations fail startup; actual main wiring is tested. Panel-native work shares one Sanaei runtime in the panels process. Cloud lifecycle/recovery/scheduler and account/network work run in control.
- Unexpected module return or panic fails that role process, with independent systemd restart. Normal cancellation joins modules; systemd bounds termination at90 seconds. No blind in-process replay of possibly committed work.
- PostgreSQL pinned-session ownership rejects duplicate roles and mixed all+split. Monitoring starts immediately after acquisition, covering migration, startup repair and normal execution; backend/lock loss ends the role. Existing durable operation/account/client leases remain authoritative.
- Shared admission BEFORE long-lived leases limits control to3 jobs and panels to10. Waiting tasks hold no SQL connection; a canceled callback retains its slot until it returns. Real PostgreSQL tests exercise nested work plus heartbeat, ownership and reconciliation headroom.
- DB pools remain control24, panels40 and API24 (total88). Each worker has independent512MiB MemoryMax,256MiB MemoryHigh,256-task and8192-descriptor limits.
- API explicitly requires both fresh role heartbeats in split mode, with no legacy fallback. Missing, stale, malformed or future progress fails readiness. Policy GATED is not reported as successful mutation.
- Install/upgrade preserve a selected split topology; ordinary new installation keeps the legacy all-role default unless configured otherwise.

## Review and verification
After credit recharge the Development Orchestrator genuinely resumed PLAN and completed IMPLEMENT/TEST/VERIFY/CHECKPOINT at 951ea1a5852e716d09f4362e75f1a4ba51ca3c7a. Initial OpenAI source review requested changes to startup ownership monitoring and DB admission headroom. Both were corrected; final static review returned PASS with no findings. Its static-only limitations are retained in ACCEPTANCE JSON.
Full Go, targeted race, isolated PostgreSQL ownership/backend-loss/readiness/saturation, OS separation, shutdown join and native HTTP response-loss/no-duplicate regressions passed.
The actual worker binary ran in a random isolated bulk_test schema with empty accounts and loopback-only fixture services. Killing fixture panels caused one automatic restart while control PID stayed unchanged; duplicate role startup was rejected. Fixtures/schema/secrets were cleaned. No production crash was injected. The initial psql search_path observer failure is retained alongside the corrected PASS.

## Production evidence
- All three services active, both role locks and exact running binary hashes verified; no unexpected restart or new global gate closure in the recorded window.
- 37 postdeployment BULK_CREATE and36 BULK_DELETE jobs succeeded by08:18:24Z.51 owned clients deleted after deployment had lived at least595 seconds; eight sampled same-panel/inbound delete-to-create replacements were verified. These include clients born before phase2; a full newly-born600-second phase2 cycle is not claimed by this short sample.
- At08:18:29Z, actual HTTP200 public shares contained Direct53 and Residential379 lines. Both Upcloud113 current servers were READY; Desired5 preserved. Counts vary with lifecycle. This verifies output, not mobile traffic.
- Six production-only resource samples show zero SQL pool waits, advancing control scans and client successes96 to136. Admission limited active work; queued work counts are recorded and queue-wide fairness is not claimed. Exact peak connections, queued work and transient DB lock waits are recorded in ACCEPTANCE JSON.
- Account/rule/profile/protection/gate/routing/publication/proxy settings were exactly preserved. Main gate remains enabled from the user's06:23:13Z Resume. Historical05:28 failure metadata does not indicate a new closure.

## Recovery and next scope
Rollback artifacts and verified deployment are in /root/backups/dob-worker-isolation-20261007. Restore previous caf3f0d binaries/topology only after BOTH split workers stop; never overlap an old all-role worker with split roles. Preserve all current policy/settings.
Roles still share the host, PostgreSQL and network. There is no watchdog for every living-but-stuck module, queue-wide fairness, independent proxy evidence or arbitrary-bug repair. Global client concurrency1 and all finite budgets/quarantine/unknown-outcome guards remain. Future work starts from the deployed source snapshot, with bounded module progress supervision and independent health evidence as separate acceptance scopes.
