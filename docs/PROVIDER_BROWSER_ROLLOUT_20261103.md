# Provider browser rollout — 2026-11-03

User scope: three AUTO-default country priorities during account creation and
an isolated, selected-country provider browser, available from both the account
wizard and each existing account. This continues the already verified release;
it does not reimplement the feature or replay uncertain provider creates.

## Verified release

- Public stable revision: `1081be7edc22bb0cd50ccfc93852aa1650f73406`.
- Source CI: `41124609102`; independent public install/test: `41124939606`.
- Atomic promotion: `41125845769`. Fresh stable ref matches on November 3.
- Native CI covered HTTPS/WebSockets/input, distinct cookie/storage profiles,
  rejected destinations and zero direct-host requests. This is fixture evidence,
  not a guarantee that every external site's authentication accepts the browser.

## Current stages

| Stage | Status |
| --- | --- |
| Recover prior cleanup receipt | Complete; phase 3 reported complete and protected configuration unchanged |
| Fresh disk check | 25,871,650,816 bytes available; production reserve 15,128,642,621 bytes |
| Default baseline | Captured; old revision 67f91d2, schema 205, ready |
| Production backup prerequisite | In progress; 06:00 dump timed out, fresh integrity inspection passed |
| Default deployment | Complete at 10:59 UTC; revision 1081be7, schema 206, five readiness checks passed |
| Installed browser sandbox | Passed on the live host; actual Chromium returned an isolated 390 × 800 frame while all tunnel attempts were denied |
| Production deployment | Guarded continuation waiting for native backup success, then fresh baseline and bounded update |
| Actual provider-site acceptance | Pending deployment |
| Final output/preservation verification | Pending deployment |

The November 3 initial production readiness result was
`database_backup_unavailable`. Journal evidence identifies a 1,800-second pg_dump
timeout at 06:34 UTC. The database integrity inspection itself passed, with no
checksum failure or recovery-required signal. The existing native integrity and
fresh-backup service was restarted at 10:46 UTC; the fresh inspection passed at
10:47 UTC and the new dump was progressing without blockers. No health receipt
was fabricated, no guard was bypassed, and existing sealed backups were retained.

Private current receipts and copied pinned rollout helpers are under
`/root/dob-provider-browser-20261103` on `limited-lime.ptr.network`. The immutable
public checkout remains `/root/dob-build-activity-20261030/public-verified`.
The old controller remains quarantined. The backup vault on port 18443 remains
protected at PID 1176 with zero restarts. Only the default instance has been
updated so far. Its three active executables match their release hashes, with
zero restarts, and all 18 preservation checks passed. The bounded builder's
actual kernel controls were verified: 1 GiB maximum memory, 768 MiB memory high
and 100% CPU quota. The production backup continued within available resources.
All 22 expected static assets, including both dynamically loaded country/browser
modules, and the configured panel HTML match the exact published release over
the real default-instance HTTP listener.

The guarded production continuation is already running (operator PID 1478821).
It requires the native backup unit to finish successfully and the real integrity
receipt to report backup success before taking a fresh production baseline or
launching the pinned bounded update. Read `production-continuation.json`,
`production-rollout/update-result.json` and the associated private logs before
resuming; do not launch a duplicate updater. The read-only health monitor is PID
1454887, with `health-latest.json` and `health-history.jsonl` in the same directory.

`ops/release/provider-browser-live-20261103.py` is the bounded operational
acceptance helper. It follows the existing temporary administrator fixture in
`cmd/admin-acceptance`, never changes existing users/accounts/proxies, and removes
its fixture in a finally block. It verifies two real provider-browser sessions
and their separate nonroot network/mount namespaces, saving only root-private
frames and a redacted receipt. It does not create paid resources or claim an
ordinary-user login test. Native CI remains the cookie/storage proof.

Browser limits remain explicit: disposable sessions, 15-minute lifetime, two
minutes without control-channel activity, and no uploads/downloads. Explicit
country priorities never acquire an arbitrary AUTO fallback; uncertain route
proof blocks launch or closes an existing session.
