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
