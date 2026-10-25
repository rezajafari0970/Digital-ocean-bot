# Restarted system audit — 2026-10-25

The user requested a new review from the beginning. The immutable baseline is
c41ee01ae47ac0c26a6db84ce951986f01a7d716. Earlier verified fixes and incident evidence
are retained. This is an ongoing audit, not a certification of permanent or
zero-error operation.

## Coverage and acceptance stages

The baseline contains 6,491 tracked files, including 1,218 Go files / 146,345 lines
(including tests). This checkpoint fully reads 42 baseline Go files / 6,727 lines,
plus six supporting or newly authored files: 48 files / 7,759 current lines total.
Search matches and partial reads do not count as full-file inspections.
SYSTEM_AUDIT_RESTART_20261025.json records every baseline Go file, its blob/hash,
module, line count and pending/read status, plus exact current hashes of inspected
files. These counts describe inspected source, not successful live acceptance.

| Stage | State at final checkpoint | Acceptance requirement |
| --- | --- | --- |
| Baseline, access and runtime identity | Verified | Exact source/release identity, original quarantine, both readiness endpoints |
| Entry points, ownership, supervision and resource admission | 42 baseline Go files read | Broader lifecycle/failure tests and bounded-load evidence still required |
| Database migration history | Fixed; both legacy cases pass independent PostgreSQL regression | NULL/empty checksum sealed, no replay, later drift rejected, data retained |
| API review durability and consumption | 23 fake-transport tests pass; original ledger migration verified | Receipt/poll recovery, durable result, unchanged USD2/three-attempt cap |
| Provider drivers, panel modules and user flows | Shared boundaries partly read; individual implementations pending | Per-driver ownership, unknown outcomes, quotas, cancellation and native API tests |
| Database restore, service outage and sustained load | Broader drills pending | Isolated restore/failure evidence, measured recovery time and resource bounds |
| Source CI, GitHub release and installation | Passed; source PR7 and public6c92d419 published | Exact source gate, full secret scan, independent fresh/repeat installation |
| Default installation rollout | Completed; exact 6c92d419 verified | Three running hashes, five readiness checks and 14 preservation checks pass |
| Production rollout | Disk reserve blocked | Restore sufficient headroom before ordinary guarded updater |

## Migration checksum defect

Runner.Up recognizes both SQL NULL and an empty string as missing legacy
checksums. Its previous UPDATE matched only NULL. An empty checksum therefore
remained empty on every startup, allowing subsequent migration-file drift to
escape the checksum mismatch check for that version.

The UPDATE now seals either missing representation. Already-applied SQL is never
replayed. The new real PostgreSQL regression covers NULL and empty legacy rows,
idempotent unchanged startup, altered migration rejection, unchanged recorded
history and retained fixture rows. It is mandatory in the shared publication
gate, increasing required regressions from 192 to 193. No migration file, schema
version, production policy or account credential is changed.

This seals the reviewed file at the first trusted backfill; it cannot prove that
an unrecorded historical file was never changed before its checksum existed.

## Resumable bounded Astra reviews

The previous synchronous request could run beyond the local 300-second timeout
without leaving a provider response ID. The helper now submits one background
Responses request and saves the returned response ID before polling. Each poll
is one GET with a 30-second network timeout, and is independently restartable.
No polling path submits another generation request.

The full maximum cost is atomically persisted and fsynced before POST. A terminal
response is durably saved before reducing its reservation to an upper-bound usage
estimate. A crash between those writes can recover from the saved result without
another API request. Private permissions, symlink rejection, a nonblocking
process lock, strict ledger/receipt validation and result digests protect the
accounting path. Bad usage, mismatched model/tier or ambiguous submission retains
the reservation and blocks new paid submissions.

Model remains gpt-6-astra with reasoning.effort=max, default tier, explicit
30-minute cache TTL, at most 64,000 input bytes and at most 16,384 output tokens.
The conservative pricing ceiling remains USD13/M input and USD50/M output.
Total cap remains USD2 and three submission attempts. There is no override for
an unresolved request, no automatic POST retry and no ledger-reset command.

Background requests explicitly use store=true so a known receipt can be retrieved
beyond the temporary background polling window. Send only reviewed, secret-scanned
public source; never production settings, credentials or customer data. This is
an intentional retention change from the old store=false request.
Reference checked: https://developers.openai.com/api/docs/guides/background

Commands are explicit: migrate, status, submit, poll and result. The canonical
ledger remains /var/lib/datacenter-bot-audit/openai/ledger.json on the ORIGINAL
host. Migration from schema1 to schema2 creates private
ledger.schema1.evidence.json and preserves all charges/reservations. Do not create
a replacement ledger on the recovery host. An unknown legacy attempt remains
unknown after migration, and its original prompt cannot be resubmitted.

The existing 11:14:16UTC request still has no receipt. Its actual tokens/cost are
unknown and USD1.501011 remains reserved. This implementation checkpoint made
no additional paid request. Resolve provider usage before another paid review.
The fake-transport tests are not a completed Astra model review.

## Architecture findings and limits

- API and the two worker roles use separate connection pools, with budgets
  24 + 24 + 40. Work admission precedes long-held SQL leases; cancellation does
  not release a slot until its handler actually returns.
- Worker ownership combines PostgreSQL session locks with a process-lifetime
  local lock. The local lock explicitly assumes one worker host. Multi-host
  active/active processing still needs a separately reviewed distributed fencing
  design; installing on multiple servers does not establish that property.
- Supervision distinguishes idle/waiting from executing stages, bounds tracked
  nodes, detects stalled work and delegates retries to a fresh process after
  exit. Native operations keep their durable ownership/reconciliation contracts.
  This is bounded recovery of known states, not arbitrary AI repair of new bugs.
- Proxy traffic attribution has a bounded map and an explicit overflow bucket.
  Its recorder owns an independent lifetime and a final flush; cancelling the
  worker startup context does not silently terminate it.
- Migration retention snapshots, transaction checks and checksum rejection are
  present. Physical database corruption still requires quarantine, verified
  backup/restore and storage diagnosis. Do not overwrite damaged pages or delete
  incident evidence to make a health check appear green.
- API background output collection, all per-provider implementations, long-run
  fairness and full restore/fault acceptance remain ongoing review work. The
  source inspection does not establish unlimited scale or zero downtime.

## Operational baseline

At 11:29:54–55UTC the original PostgreSQL/API/workers were inactive with MainPID0.
Both recovery instances returned all five readiness checks. Default release was
80c85157; production was 5dc2c256. Recovery free disk was 4,923,973,632 bytes,
below the earlier production reserve of 6,551,766,077 bytes. A fresh dynamic
preflight is required before any production update; the reserve is not bypassed.

The retained /opt/.dob-release.bXqgHMgu is the older 8d838efa candidate.
Do not describe it as the latest update. No unrelated Telegram application data,
retained database evidence or backup vault was deleted.

Private evidence/workspace: /root/dob-audit-restart-20261025 and
/root/projects/dob-audit-restart-20261025. Independent verification and the remaining operational blockers are recorded below.


## Additional architecture coverage

Factories and capability interfaces separate provider contracts. Bootstrap still
registers the six factories at compile time, and Container.openDriver contains
provider-specific setup branches. Dynamic plugin loading and complete removal
of that core coupling have not been implemented or verified. They are separate
refactoring work, with ownership/credential regressions required before changes.

Readiness validates fresh process identity, expected role/module sets, stage
progress and database integrity watermarks. The generic Prometheus Metrics helper
currently has no production caller in cmd/internal; source inspection found no
/metrics or OpenTelemetry integration there. Existing heartbeats and readiness
must not be described as complete exported metrics/tracing coverage.

## Independent CI timeout investigation

Run40190534757 passed the initial implementation. The metadata-updated candidate
in run40190644133 passed the new NULL/empty migration cases, but the pre-existing
TestRecoveryReaperDoesNotTouchLifecycleLane hit the existing 10-second discovery
deadline after10.61s. That failed run is retained; it is not relabeled as success.

Inspection covered the discovery queries, checkpoint ownership, session release,
failure ledger and fixture lifetimes. No definite root cause was established.
The release gate now requires20 consecutive repetitions before its full193-test
run. One failed repetition rejects publication; this is not a retry-until-green
loop. The test logs elapsed discovery time and pool state on failure. Runtime
deadlines, assertions and fixture workload are unchanged.

Final source run40191489281 passed all20 repetitions and the full193-test gate,
zero skips. The first repeated scan took3.590953602s; the other19 took about
30–47ms, and the full-suite scan took30.392914ms. Cold-start overhead is a possible
explanation, not a proven diagnosis or a claimed runtime fix. Preserve this
investigation if the deadline recurs; do not just raise the timeout.

## Exact source and installable release

- Implementation: bb6e4f9fd1f0a21b58b83c2610ba6561d8012148;
  PR7 merged as a70937c8ab07266d9f1dbb8843b1b9ee7cd69b4e.
- Source CI40191489281: full Go/vet/browser checks,23 API-control tests,10 gate
  tests,20 repeated isolation checks and193 mandatory DB/native/race tests,
  zero skips in the required gate.
- Public release6c92d419fa1f48cbb06fe823fb9170b7b1351198 contains1,725 files.
  Both current live-secret comparisons and gitleaks passed the full export.
- Public CI40191888240 passed exact source validation, fresh Ubuntu24.04 amd64
  installation and repeated installation, all five readiness checks, ordinary
  login and installation identity preservation.
- Promotion40192190248 updated main and stable atomically with expected-head
  leases. Both refs were independently verified at6c92d419.

Evidence links:
https://github.com/rezajafari0970/Digital-ocean-bot/pull/7
https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40191489281
https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40191888240
https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40192190248

## Authoritative API ledger continuity

At11:56:44UTC the exact helper was installed in
/root/dob-api-review-receipts-20261025 on the ORIGINAL host; SHA256
 a909b3cdaf3d79eb226f09b5659c080d946cd2d861853cf838191fd9557ff2db.
The original ledger was explicitly migrated to schema2. Its schema1 evidence copy
matches the old state, every original request field and USD1.501011 reservation
are unchanged, and there is still exactly one unknown attempt. No ledger was
created on the recovery host. The earlier synchronous helper/wrapper is historical;
use the receipt-based helper only after provider usage reconciliation.

A single read-only GET to the official organization usage endpoint at12:12:26UTC
returnedHTTP403 using the existing key. No consumption record was obtained and
no ledger settlement was applied. There were zero new generation requests in
this restarted audit. Actual usage of the earlier attempt is still unknown.
The key value was never printed or transferred. The original PG/application and
publisher timer/services were reverified inactive; no heavy build/test ran there.

At12:08:10UTC a read-only query of BOTH recovery databases found195 migrations,
zero NULL checksums and zero empty checksums, schema000195_provider_snapshot_latest.
The newly fixed legacy condition was not present in either running installation.

## Default deployment and final live acceptance

The default installation was updated from 80c85157 to 6c92d419 using the ordinary
guarded updater through ops/release/run-bounded-update.sh. A private pre-update
database dump and configuration baseline were retained. The build had systemd
MemoryMax=1GiB, MemoryHigh=768MiB, CPUQuota=100%, GOMEMLIMIT=384MiB,
GOMAXPROCS=2 and GOFLAGS=-p=1. It completed successfully in about226 seconds.
A controlled service restart occurred; this is not a zero-downtime claim.

At12:19:48UTC all three default services were active with exact executable
hashes, NRestarts=0, all five readiness checks passing and all14 policy,
credential/configuration and ownership preservation checks true. Schema remains
000195_provider_snapshot_latest. The default instance has zero profile/Trojan
identities; this result is not acceptance of the production fleet.

At12:25:26UTC both recovery installations were checked again. All six running
executable hashes matched their respective manifests, both endpoints passed all
five readiness checks, and NRestarts remained0. Default runs6c92d419; production
continues to run5dc2c256. The original host's PostgreSQL, application and publisher
were separately confirmed inactive at12:19:50UTC. Its quarantine is retained.

After the bounded updater had exited and no build process remained, only three
known completed, reproducible Go caches were removed. This recovered406,007,808
bytes. No retained backups, incident evidence or unrelated application data was
removed. The subsequent read-only production preflight still failed the reserve:
4,286,414,848 bytes free against6,593,004,605 required. Its retained candidate is
the OLD8d838efa stage, not current6c92d419. The final live snapshot had
4,285,104,128 bytes free. Disk figures are point-in-time observations; allow
additional build/backup growth and repeat dynamic preflight before deployment.
No production updater or new production backup was attempted in this audit.

Private operational evidence is under
/root/dob-audit-restart-20261025/default-rollout and live-final.json.
The public acceptance JSON contains aggregate verified outcomes, not private
configuration, database contents or credentials.

## Remaining work and exact continuation

1. Continue the pending inventory from provider factories into each driver's
   ownership, retries, unknown outcomes, quotas and cancellation contracts;
   then cover panel APIs and remaining worker/account flows. Partial reads are
   separately recorded and must not be counted as completed files.
2. Exercise restore, service/process failure, dependency outages, cancellation,
   pool fairness and sustained resource growth in isolated environments. Add
   exported metrics/traces with explicit cardinality/resource bounds. Do not
   inject faults into either live installation or restart the original cluster.
3. Resolve disk headroom without removing retained recovery evidence or unrelated
   application data. Reverify latest stable and its CI, take a fresh preservation
   baseline, and run /usr/local/sbin/dob-production update through the bounded
   launcher with normal integrity/backup/archive guards. Do not use the old fixed
   candidate resume helper as a latest-release updater.
4. Obtain verifiable provider usage for the receiptless11:14:16UTC attempt before
   another paid review. Preserve the original schema2 ledger, its schema1
   evidence and the USD1.501011 reservation. No duplicate ledger, override,
   automatic retry or reset is allowed. Subsequent authorized reviews must use
   the tested background receipt/poll helper and the existing USD2/three-attempt
   cap; fake-transport tests do not constitute a model review.
5. Retain the failed discovery-timeout evidence and diagnose recurrence with the
   added timings/pool state. Twenty passing repetitions do not prove its cause
   is repaired. Original physical storage cause, live AWS acceptance, selected
   DNS capacity and gateway management remain unresolved historical blockers.

The latest PROJECT_STATE.md and HANDOFF.md entries supersede earlier entries
about API inaccessibility, ledger schema1 and default release80c85157. Historical
evidence remains intact. This checkpoint completes the documented fixes and
their release/default rollout; the full project audit is still in progress.
