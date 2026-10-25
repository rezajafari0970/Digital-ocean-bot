# Configuration deletion, database attention and TCP HTTP headers

## Verified release and scope

Product bf56dd5c311aea8393d316674fa9e5113cfcf2e6 merged by
[PR 12](https://github.com/rezajafari0970/Digital-ocean-bot/pull/12)
as 9428375577cb36403a5f9da6e7d8ad39361287e8. Canonical branch remains
checkpoint/final-e2e-20260929. Public main and stable are
64fd97ed3d25198e7400b4f5701477e94cc92896.
Both live instances now run that public commit with schema 196.
The adjacent JSON records exact verification times, preservation checks and service hashes.

This release addresses the supplied video, confirmed deletion scheduling,
database-health diagnosis and optional TCP HTTP-header profile creation.
It does not certify the whole project, arbitrary storage repair or permanent uptime.

## Video and deletion evidence

Screenrecorder-2026-10-25-19-34-51-162.mp4 is 33.43669 seconds,
864 x 1920. Eleven frames were visually inspected and 67 sampled frames
examined with OCR. Audio was not reviewed. Three deleting profiles show
73/79/66 remaining; 79 later becomes 77 over approximately six seconds.
The recording does not show the initial Delete action, so it cannot establish
end-to-end deletion latency. Sampled database badges say ready.

Live pre-change evidence at 16:17 UTC included 300 retirements:
mean 4.78 seconds and p95 6.7996216 seconds from the recorded attempt to
retirement. This excludes waiting before that attempt and is not a global SLA.
A sampled event window included 361 panel-busy and 77 panel-unavailable events.
The production queue eventually narrowed to three listeners on one expired
AWS server. Its account reports TOKEN_INVALID; native panel access is unavailable.
A timeout or invalid cloud credential cannot prove that a remote listener is gone.

Changes:

- Creation consumers also service due deletion using the existing shared admission
  and SSH budgets, allowing idle creation capacity to assist cleanup.
- Due deletion precedes routine proof work on a panel. Proof work approaching
  its publication deadline remains eligible, preserving output freshness.
- Profile tombstone completion runs independently of the creation loop,
  before and after non-verification reconciliation.
- Late failed/busy deletion writes cannot revive provider-confirmed RETIRED rows.
- Unavailable panel, inventory, identity and native runtime failure codes are
  distinguished; deletion intent and retry remain durable.

No remote object was force-marked deleted. The account credential problem is
still an external blocker for those three listeners. Correct the account through
the existing Accounts UI; do not put credentials in a report or chat.
No after-change fleet latency benchmark or percentage speedup is claimed.

## TCP HTTP-header creation

Configs -> New/Edit -> TCP -> Connection settings -> TCP header: HTTP.
Host defaults to play.googleapis.com; path defaults to /.
For the user's plain TCP example choose security None. HTTP headers add no TLS.

The Spec, driver registry, server/probe stream settings, profile API persistence,
create/edit UI and exports use the existing ownership and publication boundaries.
No client/core modification or disabling of TLS verification is introduced.
The existing warnings about compatible clients for plain VLESS/Trojan remain.

- VLESS/Trojan export headerType=http, host and path in their URI.
- VMess retains its JSON/base64 format with type=http, host and path.
- Shadowsocks retains SIP002 with its HTTP-obfuscation plugin representation.
  A VLESS-shaped query is not universally meaningful for VMess or Shadowsocks.
- HTTP headers require TCP and a valid Host/path, and reject Reality/Vision.
- Selecting None or another transport clears incompatible stale form fields.
- Seven native loopback round trips cover four plain protocols and three TLS
  variants. They do not prove behavior on every official Android client version.

## Database attention and disk reserve

The observed current readiness failure was database_disk_space_low.
The database was approximately 2.06 GB on a 60 GiB root filesystem.
Current observations did not establish new physical database corruption.
The earlier original-host storage incident still has no proven root cause.

Seven old routine release archives, totaling 5,211,867,925 bytes, were moved to
the original host through its existing restricted read-only backup SSH channel.
All destination files were SHA-256 verified and fsynced. Source size, exact mtime,
link count and content hash were rechecked before removing the recovery-host copy.
Private pointer receipts preserve the new location and hashes.
Current backups, incident evidence and unrelated collector data were preserved.
After space reclamation settled, free space was 7,751,200,768 bytes at
17:05:41 UTC after the first six archives, and both instances passed all five readiness checks.
The seventh was an older routine audit backup, relocated only after the new
614,941,190-byte pre-update production backup had completed. Fresh and incident
archives remained local.
Migration retention comparisons temporarily consumed additional space; those
temporary database files were released when migration completed. Final free space
was 8,179,658,752 bytes at 17:32:30 UTC; final JSON records the snapshot. Disk growth still requires capacity planning.

The UI now distinguishes low disk, recovery, stale checks and failed backups.
Malformed or contradictory health responses cannot show ready.
Existing deployment disk, sealed backup, identity and rollback gates were retained.
No checksum disabling, destructive repair or overwrite of the live database occurred.
The original project cluster remains down; its application workers and publisher
remain quarantined. Its generic PostgreSQL service is an exited umbrella unit,
which is not evidence that the quarantined cluster is running.

## Validation and deployment

- [Final source CI 40214540128](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40214540128):
  success on exact product bf56dd5c; full Go tests/vet/browser,
  220 mandatory roots with zero required skips, native core and race checks,
  20 isolation repetitions and disposable database restore through migration 196.
- [Public CI 40214736483](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40214736483):
  success on exact candidate 64fd97ed; fresh and repeated Ubuntu 24 amd64 install,
  ordinary login, identity preservation and all five readiness checks.
- [Promotion 40215046466](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40215046466):
  exact CI rechecked; main/stable advanced atomically with leases and verified.
- Public export: 1,740 files / 6,557,162 bytes; both live secret-set scans
  and gitleaks passed before publication.
- Both live updates ran with a 1 GiB memory cap and one CPU quota.
  Independent production staging, backup, reserve and rollback checks remained active.
- Both instances passed 15 preservation checks, all three running-binary hashes
  and all five readiness checks. Production advanced from schema 195 to 196.
  Production had a controlled service stop while migrations and retained-data
  comparisons ran: API stop-to-start was 239.57 seconds. Total update durations
  were 218.95 seconds for default and 544.10 seconds for production. This was not
  a zero-downtime deployment. HTTPS gateway version/readiness and served HTTP-header
  UI asset were verified with normal TLS certificate validation after deployment.
  Existing schedule/account/proxy/routing/Cloudflare and encrypted ownership
  evidence was compared; the adjacent JSON describes allowed lifecycle changes.
- Default has zero profile/Trojan identities; that instance alone is not fleet proof.
  Production baseline contained 6,179 profile and 77 Trojan identity records.
- No additional cloud resource was created for release tests. Existing user
  scheduling remained authoritative.
- No new paid OpenAI generation was issued. The original schema-2 ledger retains
  one unknown request with USD 1.501011 reserved under USD 2 / three attempts.
  Actual cost/tokens remain unknown; do not reset or duplicate this request.

## Coverage and remaining work

Cumulative full-current-file review: 235 files / 27,909 lines, including
22 newly completed files in this continuation. Baseline Go coverage:
188 / 1,218 files, 22,340 / 146,345 baseline lines.
Hashes were reverified in SYSTEM_AUDIT_RESTART_20261025.json.
Partial app.js, config_lifecycle.go, database-integrity.py and README reads
are explicitly recorded and are not counted as complete files.

Next: resolve the existing AWS account authorization failure and observe confirmed
remote cleanup; validate the new profiles on the user's official v2rayNG device;
measure real deletion throughput under representative healthy-panel load.
Continue the remaining whole-project audit, including application/provider
ownership and resource/observability paths. One read finding to reproduce:
internal/app/lifecycle.go confirmDeletedTx closes its row iteration without
an explicit rows.Err check; do not represent that path as fully hardened.
Original physical storage root cause and remaining API usage reconciliation
remain unresolved.
