# Scheduler durability and worker/resource audit continuation

Updated: 2026-10-25T19:29:56.573620+03:30. Continuation baseline: a924bc27b4f70453f0bbe79ef75f785774884e7e.
**Whole-project audit: INCOMPLETE.** Earlier SSH identity, encrypted-record
containment and disposable restore work is preserved in
[SSH report](SSH_IDENTITY_DURABILITY_20261025.md).
Read the [coverage inventory](SCHEDULER_DURABILITY_20261025.json),
[cumulative inventory](SYSTEM_AUDIT_RESTART_20261025.json) and
[acceptance receipts](SCHEDULER_DURABILITY_20261025_ACCEPTANCE.json).

## Stage status

| Stage | Status | Evidence and limit |
| --- | --- | --- |
| Worker, scheduler, resource, capacity source reading | Recorded | 75 additional complete files; 213 current files / 22,638 lines cumulative |
| Entire baseline Go review | INCOMPLETE | 173 / 1,218 files; 18,017 / 146,345 baseline lines |
| Scheduler ownership/error defects | Fixed, tested, merged | PR #11; product 721b02b2f40607208107e57ee62b84ba3c6f81de |
| Final source verification | PASS | CI40208116457; 219 required roots, zero required skips, 20 isolation repetitions |
| Disposable restore/interruption | PASS | 196 actual migrations; no live database accessed |
| Public installer | Published | main/stable dfe97084c84ad617609cafbff210b1e9fa31dc45; exact fresh/repeated install passed |
| Default installation | Updated, verified | Schema 196; all 15 preservation checks and 3 executing hashes |
| Production installation | BLOCKED: disk reserve | Still 5dc2c256; no update attempted |
| Original host | Quarantined | Database/application/publisher inactive |
| Paid API review | BLOCKED: unresolved request | One original unknown request; zero new generation calls |

## Reproduced scheduler defects and correction

An isolated PostgreSQL fixture at 6d697dbf reproduced five failure classes in
[negative CI40206951737](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40206951737):
stale completion cleared a replacement lease and advanced its cadence;
an interrupted row iterator returned a partial claim and committed it;
failed account preparation still dispatched creation;
failed pending-capacity observation still dispatched creation;
and failed completion was returned as a successful round.
The healthy fixture passed. No real provider account or live database was mutated.

Each claimed schedule now receives a database-generated UUID ownership token and
lease expiry. Claim selection is bounded to 100 rows and checks complete iteration
and close before writing claims. Partial reads roll back. Profile/account bindings
are checked. Completion compares schedule, account, profile and token; an older
owner cannot clear a replacement claim. Legacy SQLStore.Advance uses the same
conditional completion, and a read-only Due result cannot complete a claim.

RunDue validates cancellation and current ownership before work and before each
deployment dispatch. Its per-claim context cannot outlive the lease. SQL observation,
account preparation and completion errors propagate; incomplete capacity evidence
does not authorize creation. Independent claimed accounts continue after an error,
but the round reports the error rather than successful progress. Completion has
a bounded three-second context after cancellation and retains token fencing.
No SQL transaction stays open across provider calls.

The claim batch remains capped at 100 with a one-minute lease. Queued claims that
expire are refused rather than silently extended. This does not establish
multi-host exactly-once external execution: application/provider operation fences
and authoritative provider evidence remain necessary.

## Migration and mandatory regression

Migration 000196_scheduler_claim_fencing adds nullable UUID lease_token.
Its down migration rejects every unsettled token, including an expired claim.
Settle ownership through the ordinary completion/recovery path before rollback;
do not clear tokens manually to force a downgrade.

Four new required PostgreSQL roots cover stale completion, partial claim reads,
read/write failure propagation, cancellation, replacement, expiry, disabled claims,
concurrent one-winner acquisition, account/profile mismatch and guarded migration
rollback. The fixture uses a private schema and synthetic deployment starter.
The final test also calls legacy SQLStore.Advance, closing that ownership bypass.

[PR #11](https://github.com/rezajafari0970/Digital-ocean-bot/pull/11) merged as 9d24af6889d186c59a7b9493035bfded13a42107.
Exact product 721b02b2f40607208107e57ee62b84ba3c6f81de passed
[CI40208116457](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40208116457):
full Go/vet/browser checks, 219 required database/native/race roots with zero required
skips and 20 unchanged recovery isolation repetitions. Earlier implementation
CI40207533842 also passed; the final CI covers the subsequent legacy Advance fix.

The disposable restore drill applies all 196 migrations, restores sealed backup
plus synthetic configuration key, verifies actual encrypted identity, unknown
SSH intent, sequences and pg_amcheck, rejects a damaged archive atomically, and
checks rollback after terminating an uncommitted transaction.
This is not a retained production-backup restore, physical storage repair,
a long-running load experiment or an RTO measurement.

## Source coverage and architecture findings

Every counted file was fully read and SHA-256 reverified. Search matches, test
success and partial reads are not counted as complete review. The baseline stays
c41ee01ae47ac0c26a6db84ce951986f01a7d716: 6,491 tracked files, 1,218 Go files,
146,345 Go lines. New test/migration/support files belong to current coverage,
not the immutable baseline denominator.

Current full module reading includes cmd/worker 22/22, internal/worker 33/33,
internal/scheduler 8/8, resources 6/6, resilience 5/5, jobs 5/5 and capacity 4/4.
Complete reading is not an error-free certification. Existing admission budgets
bound shared SQL ownership and retain permits through joined cancellation.
The existing fleet simulations and bounded real-time fixtures are not a new
sustained-load acceptance for this continuation.

No production reference to legacy jobs.Queue was found; it has append/length
behavior without a consumption/bounded-capacity contract. Generic resilience
Retrier/Circuit also had no production caller found. Cancellation before a first
call, concurrent half-open trials and extreme duration arithmetic need explicit
contract tests before adoption. These are source-review concerns, not reproduced
live incidents or newly implemented functionality. Compile-time factories and
provider-specific core branches still exist. Fully independent dynamic plugins,
global bounded metrics/tracing and multi-host fencing are not certified.

## Export, publication and default rollout

The 1,738-file export passed scans against both live configuration secret sets and
gitleaks before GitHub writes. Canonical manifest SHA-256:
2182c3a8a4e3b30e95553b00b1633bab945ec8b03ef18821a93b10fae68175fc.
Public [CI40208618676](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40208618676)
passed source checks and exact fresh/repeated Ubuntu 24.04 amd64 installation.
Each installation receipt verified 5 readiness checks, ordinary login and preserved
identity at dfe97084c84ad617609cafbff210b1e9fa31dc45. Promotion
[40209025769](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40209025769)
rechecked candidate CI/manifest and atomically advanced main/stable with exact
old-head leases. Both actual refs were independently verified afterward.
The reviewed one-time promotion workflow is retained under ops/release.

Default update took approximately 225.40s from persisted intent to result.
MemoryMax=1GiB, MemoryHigh=768MiB, CPUQuota=100%, GOMEMLIMIT=384MiB and GOMAXPROCS=2
bounded the ordinary updater. Controlled service restarts occurred.
At 2026-10-25T15:58:19.932759+00:00, schema 196, all 15 preservation checks,
three executing hashes and 5/5 readiness checks passed. Schedule policy is now
included in preservation checks; operational claim/cadence fields are excluded.
There are zero default profile/Trojan identities, so this is not production fleet
acceptance. At 2026-10-25T15:59:17.820075+00:00 both installations had all six
services active with matching executing hashes and 5/5 readiness each.

## Production, original host and API blockers

Production remains 5dc2c25678698dbbe9ac1cfee7d18701c19213e7 on schema 195.
Read-only preflight at 2026-10-25T15:59:18.272845+00:00 found
3,828,371,456 bytes free against 6,752,797,757 required:
a deficit of 2,924,426,301 bytes before additional build/backup growth.
No production update was attempted and no deployment is pending.
After the verified update, go clean -cache reclaimed 297,222,144 bytes of reproducible build cache.
The 60GiB disk is fully partitioned. Retained incident archives, rollback backups
and unrelated application data remain intact. The next daily backup also requires
5GiB free; insufficient future headroom is not proof a scheduled backup failed.

Restore sufficient space through an authorized storage expansion or explicitly
identified disposable data. Then reverify latest stable/CI, capture a fresh
production preservation baseline and use the ordinary /usr/local/sbin/dob-production
update via ops/release/run-bounded-update.sh. Preserve archive/integrity guards.
The retained /opt/.dob-release.bXqgHMgu is OLD 8d838efa; do not deploy it as latest.

At 2026-10-25T15:55:02.861044+00:00, original PostgreSQL/API/workers and
datacenter-publisher remained inactive. Keep quarantine and avoid heavy tests there.
The existing API key stays private on that host. Its authoritative schema 2 ledger
retains one unknown request with USD 1.501011 reserved under USD 2/three attempts.
Actual tokens/bill remain unknown; zero new generation requests were sent here.
Do not reset, duplicate or override the ledger. Resolve the original provider
usage/result before another paid review. No independent paid API review or model
switch is claimed by this continuation.

## Remaining acceptance and next work

Continue complete provider/panel/application source and ownership inventories;
prioritize production-reachable paths with reproducible defects. Add targeted
fault/load tests where contracts require them. Finish cross-module plugin and
resource/observability contracts, active-active ownership analysis, retained
production-backup restore on adequate resources and original storage-cause work.
Separate real AWS/DNS/gateway/fleet acceptance from synthetic tests.

The whole project, permanent uptime and automatic repair of arbitrary future
damage are not certified. Preserve evidence and refuse ambiguous destructive
recovery. This checkpoint records completed work and blockers, not background
execution after the session.

## Release scan boundary correction

A missing private PR-body directory unexpectedly passed the previous live-secret
scanner and gitleaks scanned zero bytes. No PR body was published from that failed
write. The existing export and checkpoint scans had real nonempty source trees.
The private scanner now rejects missing/non-directory/symlink roots, empty or
zero-byte trees, unreadable traversal, nested symlinks and special files before
reading live configuration. Successful receipts include file and byte counts.

Eight synthetic boundary tests pass locally and in independent
[CI40210555780](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40210555780)
at265498ddd85d4c92a36666902ce4ecc46724dcec. A minimal workflow keeps these tests required
on relevant pushes/PRs. The exact public export was rescanned successfully against
both live configurations and gitleaks:1,738 files,6,533,511 bytes. This operations-only
change does not change the installed product or exported application.

## Active follow-up requested during this checkpoint

The user supplied Screenrecorder-2026-10-25-19-34-51-162.mp4 and requested fast,
confirmed configuration deletion, investigation of intermittent database needs
attention and TCP HTTP-header configuration creation with host play.googleapis.com
for compatible VLESS/VMess/Trojan/Shadowsocks formats. These are active work, not
completed features in this scheduler release.

Sampled video observations: three deleting profiles show73,79 and66 pending listeners;
the79 count later reaches77. One active profile shows101 created per direct/residential
class but65 published per class. Visible readiness badges are ready. The video alone
does not establish root cause or physical database corruption. Correlate bounded
live SQL/worker evidence and actual client/core format contracts before changes.
