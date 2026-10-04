# Production recovery controls — 2026-10-04

Continuation source: checkpoint/final-e2e-20260929, parent 188e642ac449e62ef155a3356a44a0fed5cec33f. Production initially ran af7e34c250438379854d550fcbe53dee837c1db5.

## Observations

Reviewed the entire 136.8-second recording Screenrecorder-2026-10-04-12-40-28-841.mp4 at one-second intervals, including account deletion at 4s/56s, billing sections, Residential at 96s, cleanup/resume controls at 104–114s, and empty Output at 126–132s. Audio samples were silent (-91 dB).

Fresh production on Oct 4: 37 active servers, one failed deployment, 20 pending-deletion servers. Cleanup 7a105e85-0978-4149-91b1-76baffb5b692: 38 succeeded, 15 failed, policy disabled at revision 22, mutation gates closed, Output empty. All 15 unresolved targets belong to the retiring Vultr account whose token is invalid. Do not report these as serving-fleet routing failures.

Failed deployment e3cfae26-7a94-4e2f-8d8c-deafefa94768 had FAILED panel configuration while droplet 5fb1cc40-ca08-4df6-af77-3a53e3b59ae0 remained PROVISIONING. PostInstallWorkflow omitted FailureFinalizer. Read-only SSH confirmed x-ui was running later; that does not establish the original command succeeded. Panel setup previously waited only two seconds before its readiness probe.

## Changes

- Explicit POST /api/v1/config-capacity/restore checks policy revision and the exact active cleanup ID under executor/global/lifecycle locks; stops remaining cleanup, retains deletion audit, enables the saved policy and eligible global lifecycle scopes without resetting budgets/expiry. It refuses to widen a scoped canary gate. Continuous execution remains concurrency=1 with existing worker fail-close.
- The legacy bulk gate deliberately remains closed. Durable BULK_CREATE/BULK_DELETE are executed by the client mutation journal worker.
- Saved cleanup resumes through /cleanup/{id}/resume. A stale completed-job button cannot initiate a new fleet deletion. Repeated initial planning does not reset failed targets; explicit resume is required.
- Cleanup freeze increments policy revision. UI distinguishes continuing deletion from restoring creation, disables incompatible controls and refreshes cleanup/capacity status.
- Account refresh and delete feedback preserve billing expansion and scroll position. Errors display in a fixed visible toast.
- Provider-blocked account deletion checks back off to 30 minutes. Explicit retry or a validated replacement token schedules an immediate check. No credentials or resource records are purged before provider absence is verified.
- Serving-fleet residential statistics exclude retiring/deleting accounts and report those separately.
- Browser-only Output view (?view=1) displays an explicit empty/error state and polls fresh plaintext every second. Subscription format, immutable class boundaries and freshness/expiry filters are unchanged.
- Post-install terminal failures invoke the existing failure finalizer. Bounded recovery (at most 8) handles previously stranded, never-ready provisioning droplets whose deployments are durably terminal. READY/rearmed deployments are excluded. Existing lifecycle handles provider deletion and replacement.
- Panel startup readiness polls read-only for a bounded period without repeating settings or restart within that wait.

## Verification

Focused and race tests use isolated PostgreSQL schemas in dob_bulk_test_20261003, never production fixtures. Tests cover restore stale revision/cleanup/gate rejection, in-flight mutation serialization, budget preservation, response-loss duplicate protection, terminal failure retirement, idempotent crash recovery, preserving READY/rearmed servers, routing counts and share class boundaries. Shell fault tests simulate cold startup and permanent unavailability.

OpenAI server adapter reviewed the bounded source patch. Review findings were independently checked: legacy bulk gate closure and global auto-enrollment are intentional; scoped-gate protection, freeze revision, commit outcome and duplicate-retirement safeguards were addressed.

Evidence: /root/backups/dob-regression-20261004. Production acceptance is recorded separately after deployment; this document does not claim completion.

## External blockers

Three pending DigitalOcean deletions are provider-locked; one Vultr deletion has an invalid token. Twenty tracked provider servers remain. Retain credentials and audit until fresh provider verification becomes possible. A working Delete button does not override provider access restrictions.

## Final production acceptance — 2026-10-04 10:24 UTC

Runtime source: b067c364e91355055c63a00977dec4fa1dceb0df, built from a clean detached checkout. API and Worker are active, modified=false, zero automatic restarts. Latest successful deployment evidence is deploy-memory-atomic.log/status in the evidence directory; the earlier deploy.log records a deliberately failed acceptance check for the Output script path, subsequently fixed.

See PRODUCTION_REGRESSION_ACCEPTANCE_20261004.json for machine-readable evidence. Final observations: 38 active servers, zero inactive/broken servers, 38/38 serving routing proofs, zero panel-attention servers; 76 fresh configs, 38 DIRECT and 38 RESIDENTIAL, no overlap. Both actual VLESS/Reality egress classes passed on the final small-memory replacement.

Saved Reality policy is enabled at revision 23: target=2, rate=1, quota=0, lifetime=10800, both classes enabled. Main durable gate is intentionally enabled, global scope, concurrency=1; legacy bulk gate remains disabled with kill switch set. There are 39 enabled lifecycle scope rows, 38 serving panels, 38 new succeeded BULK_CREATE jobs, no new unresolved jobs. The two historical failed CREATE jobs remain unchanged.

Cleanup 7a105e85-0978-4149-91b1-76baffb5b692 is CANCELLED. Its 38 completed and 15 failed target records remain for audit. Restore was observed committed at 09:39:36 UTC while this recovery was in progress; attribution of that browser action was not established. Production replay of the stale revision and terminal cleanup resume both returned 409 without changing the enabled policy or creating another cleanup.

### Additional causes established during acceptance

- Provider observations sometimes arrived 121–204 seconds apart despite a 120-second dashboard freshness boundary. Worker refresh now starts at age 60 seconds on a 15-second cadence; the freshness boundary and provider error backoffs are unchanged. Twelve consecutive samples showed 38 active/zero inactive servers and maximum provider observation age 71 seconds.
- Runtime repair used the expired worker context to persist its result. Failed attempts accumulated with an empty error and did not reach the existing retirement policy. Completion now uses bounded independent verification/persistence contexts and an atomic ledger/lifecycle transaction. A fresh successful panel observation reconciles a lost repair response as success.
- The next replacement reproduced memory pressure: 453 MiB RAM, no swap, two kernel OOM kills of apt-get, and an unresponsive panel API. The final replacement has a root-owned, journaled 1 GiB swap file, responsive API and two freshly verified clients. Memory preparation runs before bootstrap/package steps and panel configuration/repair on qualifying small hosts. Existing unrelated swap/files are preserved; insufficient disk, unsafe paths or missing fstab fail closed.
- Managed fstab persistence is atomic, preserves unrelated entries, verifies ownership/type and rejects conflicting/duplicate managed entries. Fault tests cover committed swapon with lost response and interruption before fstab replacement. No firewall or safety controls were disabled by this memory repair.

Old problematic droplets 5fb1cc40-ca08-4df6-af77-3a53e3b59ae0 and 81e07bbb-26b8-46d6-9ad4-ba5bce290929 were provider-verified absent. The first replacement 37637c3b-0dbb-4db5-9dff-d5441290946f succeeded. The intermediate second replacement e98c2316-32bc-4cb3-a423-258e76af91b0 reproduced OOM and was retired; final replacement 54e88c88-962c-492f-9901-63ecd11a1270 is PANEL_COMPLETE with panel 69e68a2a-8a42-4c9f-9f12-302920d729bc, healthy routing and real traffic verified.

### Tests and continuation

Focused PostgreSQL recovery tests, race tests, clean full Go test suites, mobile browser regression controls, actual account Delete fixture/purge, Residential/Proxy isolation fixtures, stale request rejection and real traffic passed. Fixture accounts and residential entries were independently confirmed absent afterward. OpenAI review findings on fstab crash consistency and readiness invariants were independently verified and corrected before the final deployment.

The remaining 20 pending servers belong to A1/A2/A3 (DigitalOcean LOCKED: 3/1/1 servers) and New 305$ vv (Vultr TOKEN_INVALID: 15). Restore provider access before verifying absence and purging their records. Do not erase credentials/audit merely to hide these pending operations.

Real quota-exhaustion acceptance was not rerun during this recovery. The existing lifecycle canary requires the fleet policy disabled; do not run it blindly against the recovered live fleet. Prepare a separately scoped acceptance/recovery plan that preserves continuous production creation. Historical 10000-client and earlier lifecycle runs remain historical accepted evidence, not new tests performed today.
