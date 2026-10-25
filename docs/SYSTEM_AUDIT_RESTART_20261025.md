# Restarted system audit — 2026-10-25

The user requested a new review from the beginning. The immutable baseline is
c41ee01ae47ac0c26a6db84ce951986f01a7d716. Earlier verified fixes and incident evidence
are retained. This is an ongoing audit, not a certification of permanent or
zero-error operation.

## Coverage and acceptance stages

The baseline contains 6,491 tracked files, including 1,218 Go files / 146,345 lines
(including tests). This checkpoint fully reads 21 baseline Go files / 4,002 lines,
plus six supporting or newly authored files: 27 files / 5,032 current lines total.
Search matches and partial reads do not count as full-file inspections.
SYSTEM_AUDIT_RESTART_20261025.json records every baseline Go file, its blob/hash,
module, line count and pending/read status, plus exact current hashes of inspected
files. These counts describe inspected source, not successful live acceptance.

| Stage | State at implementation checkpoint | Acceptance requirement |
| --- | --- | --- |
| Baseline, access and runtime identity | Verified | Exact source/release identity, original quarantine, both readiness endpoints |
| Entry points, ownership, supervision and resource admission | 21 baseline Go files read | Broader lifecycle/failure tests and bounded-load evidence still required |
| Database migration history | Defect fixed; independent PostgreSQL regression pending | NULL/empty checksum sealed, no replay, later drift rejected, data retained |
| API review durability and consumption | 23 fake-transport tests pass | Receipt/poll recovery, durable result, unchanged USD2/three-attempt cap |
| Provider drivers, panel modules and user flows | Shared boundaries partly read; individual implementations pending | Per-driver ownership, unknown outcomes, quotas, cancellation and native API tests |
| Database restore, service outage and sustained load | Broader drills pending | Isolated restore/failure evidence, measured recovery time and resource bounds |
| Source CI, GitHub release and installation | Pending for this candidate | Exact source gate, full secret scan, independent fresh/repeat installation |
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
/root/projects/dob-audit-restart-20261025. Final CI, release/deployment results and
the exact next action will be appended after independent verification.
