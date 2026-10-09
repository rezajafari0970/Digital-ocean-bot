# Bounded residential stability pilot (2026-10-09)

**Release status: DEPLOYED / service health verified / production field pilot pending.**
This document describes the deployed capability; no new production exclusion trial has been accepted.
Baseline: 94b1f4dfbba3d3d71632fe276c61a5994f53ed10.
Current production: f1ae87dc66dfbba26d20ff3931791ee01f795390.
Rollback source: fc76e6a669931873295ddded4f51eca3a7bbe8ca, only after safe restoration checks.
Job: residential-stability-20261009.

## Problem and scope

The existing native pool observes a gstatic 204 endpoint; this does not establish
Ads/GPT availability. Independent observations showed gateway outcomes changing
without endpoint/configuration changes. A single historical failure cannot justify
a permanent or fleet-wide exclusion.

The candidate is a one-shot operator supervisor for one panel, one or two fixed
suspects and two fixed controls. It may authorize at most one logical five-minute,
exclusion-only trial through the existing Go Store. It does not install a daemon,
publish a fleet profile, extend a deadline, replace control roles, rotate a provider
session, relax the allowlist, or manufacture advertising events.
Automatic fleet quarantine/readmission is a separate future feature.

## Frozen qualification

Before measurement, --freeze writes a private policy with exact parent, selected
panel identity/generation/native plan, other assignments, policy/proxy-version
fingerprints, runtime manifest and SHA256-pinned operator tools. Qualification is
bounded to twenty minutes. Parent must be KEPT/permanent/fleet, 13/80, without
exclusions; selected server must have fresh native proof and sufficient lifetime.

The baseline is native route proof plus a fixed 18-case actual traffic window.
Each trusted immutable admission receipt contains three rounds of four
destinations per gateway. Both independent windows must qualify with identical
roles/context; window two starts at least sixty seconds after window one finishes.
A failed control or ineligible suspect stops the job. No third window, merged
samples, replacement controls or retry-until-green is permitted.

The optional Request.stability_evidence_id adds an atomic Go admission gate:
- first receipt immediately precedes the second; second is the latest for this panel;
- first oldest observation is at most ten minutes old, second at most five minutes,
  measured using database wall clock under the existing performance transaction lock;
- both receipts satisfy existing collector, completeness, context and role rules;
- each suspect fails the same Ads or GPT destination three times in each window;
- an already used admission receipt cannot be reused.

The first ID participates in the canonical Go request hash. No database migration
is needed. Omitted optional field preserves legacy request bytes and manual
single-receipt admission semantics. Old tune binaries use DisallowUnknownFields
and reject the new request rather than silently ignoring its first-window field.

## One operation, durable recovery

The supervisor records and fsyncs the exact request/UUID before submission.
The stored request must still equal the frozen one-panel, five-minute,
exclusion-only contract. One ambiguous operation can be replayed at most once,
using its identical UUID and payload after an authoritative read. The Go Store
checks the full request hash. Read-only reconciliation checks committed owner,
version and evidence, then the owned tuning record and current assignments.

No probe window is repeated after restart. An interrupted qualification ends
NO_ACTION. An interrupted unaccepted trial is rejected and, if still owned,
cancelled through the supported CAS operation. A new owner is never cancelled.
Cancellation gets one durable exact request; CAS drift never creates another.
An unknown outcome remains RECOVERY_PENDING; it is not asserted to have failed
or succeeded. Normal server workers retain the durable deadline recovery duty.

Candidate application AND the complete106-case native check share one ninety-second
budget anchored to database trial deadline minus five minutes. All actual traffic
proof must finish before the original five-minute deadline,
with owned tuning/generation checks around measurements. Failure requests owned
cancellation. Acceptance waits for automatic deadline restoration.

COMPLETE means exact parent restoration, generation base+2, fresh APPLIED/native
proof, equality with the frozen parent plan hash and a successful final route check. trial_outcome is independently accepted
or rejected. Deadline expiry alone never means restoration. Observation is bounded
to deadline+15 minutes (also a twenty-minute monotonic loop bound); otherwise
RECOVERY_PENDING remains visible.

The native checker uses 106 cases for this reviewed pool topology:
76 class/network/destination cases, two special-inbound cases and 28 inner-pool
cases. This count does not depend on whether one or two candidates are excluded.
A missing lane or changed topology does not pass this frozen contract.

## Traffic acceptance

All eighteen unique expected cases must be present; duplicate/missing/invalid
results fail. The local Xray process must remain healthy.

| Group | Required evidence |
|---|---|
| Three client probes | HTTP204, curl0, existing direct VPS path |
| Three denied destinations | HTTP0 and curl35/52/56, with live local core |
| DIRECT positive control | HTTP200, curl0 |
| RES/DIRECT BrowserLeaks egress | Both HTTP200/curl0, distinct hashed IPv4 |
| Nine static Ads/GPT/BrowserLeaks HEADs | At least8/9 success and no fewer than baseline |

Successful static latency must be finite and positive. Candidate successful-static
median must not exceed max(2 times baseline median, baseline median+0.5 seconds).
A baseline with no successful static requests is unusable. These modest transport
criteria are not an ad SDK show-rate, fill-rate, physical Android, or sub10ms claim.
Every failed attempt remains in its original report.

The fixed collector allows no arbitrary URLs. It uses paired fresh Output clients,
local private Xray config, at most two concurrent requests, no curl retries,
six-second request deadlines, HEAD for static resources, and BrowserLeaks GET
only to derive hashed egress. It does not click or request an ad impression.

## Artifacts, locking and progress

Run directory and staging directory are root-owned mode0700. Policy/state/status
writes use temp0600, atomic replace and file/directory fsync. Reviewed regular
artifacts are copied into private read-only staging and their hashes rechecked.
Recovery can use its intact staged copy after the original source is unavailable.
Credential/config/DSN values are never included in status or public evidence.

One host-wide exclusive supervisor flock prevents concurrent jobs. A shared
deployment flock prevents release replacement during the bounded workflow.
Children use bounded execution, process-group cleanup and parent-death signals.
Do not kill a worker or bypass recovery to release a lock.

Status contains phase/reason, actual PID/start ticks/boot ID, UTC and Tehran times,
separate trial/restoration results, and a ninety-second activity lease.
Subprocess polling refreshes at most every fifteen seconds. --status is read-only
and checks process identity plus lease. An expired lease means activity unconfirmed.
Terminal states and RECOVERY_PENDING have no active lease. No background AI is
implied; application workers are separate.

## Operator sequence after release gates pass

1. Finish the independent plan/code/adversarial review and resolve all findings.
2. Preserve focused/race/full/vet/Python fault evidence and a source checkpoint.
3. Release through the existing reviewed deployment path and verify live hashes.
   Rebuild tune/admission/native tools from that accepted source; use reviewed Xray.
4. Choose a panel with enough remaining lifetime. Freeze fixed suspects/controls
   based on retained evidence before measuring. Supply a JSON mapping of absolute
   paths with exactly admission, tune, native, traffic, support and xray keys.
5. Run --freeze, then --run on that private directory using protected service env.
   --status never restarts probes. Reuse --run only to reconcile that same job.
6. Preserve both qualification receipts and all outcomes. A NO_ACTION result does
   not permit a fresh job merely to chase a passing sample.
7. Prove exact restoration before considering the production trial accepted.

Command interface:
    python3 tools/residential-stability-pilot.py --freeze --directory /private/job \
      --panel UUID --suspects UUID[,UUID] --controls UUID,UUID --tools /private/tools.json
    python3 tools/residential-stability-pilot.py --run --directory /private/job
    python3 tools/residential-stability-pilot.py --status --directory /private/job

Do not run the candidate against production until the current independent review and deterministic release gates pass.
Rollback to fc76e6a is allowed only after all active tuning/exclusion obligations
are exactly restored and verified by the existing deployment compatibility gate.

## Verification boundaries

Python fake-adapter tests exercise state-machine/fault contracts. Real child
timeout and cross-process flock tests exercise process primitives. Safe recorded
traffic fixtures exercise the oracle. Go tests use an isolated PostgreSQL database
for authorization, concurrent starts, request replay, assignment isolation and
deadline/restoration. These are compositional evidence, not a successful new
production trial. Shared provider implementations and native planner are unchanged.

Remaining external evidence: accepted exclusion pilot; provider-path root cause;
physical Android/v2rayNG timings; app SDK load/show/impression events; prior
IP+SNI/UpCloud gaps. The canonical detached-bootstrap warning remains recorded.


## Independent-review repair record (continuation resume-1327)

API credit was restored and all four independent lanes responded. The first
source review returned REVISE; fixes are being reviewed against the new source,
not presumed accepted because old tests passed.

- F1: every uncommitted start/replay checks frozen runtime again; committed recovery
  does not require the old runtime to remain installed.
- F2: --reconcile uses a read-only Go transaction and Store.Do's shared complete
  canonical request hash. Owned tuning evidence/base fields also have to match.
  Legacy hashes are generated from deployed fc76e6a source and tested, including
  committed operation replay. Old CLI rejects the optional unknown field.
- F3: disappeared unselected assignment requires affirmative DELETED inventory;
  missing inventory or a missing live assignment fails closed.
- F4: regular-file stdin cannot block on pipe capacity. A dedicated Linux subreaper
  closes parent-death registration races, retains process locks through cleanup,
  kills the tool group even after its leader exits and escalates ignored SIGTERM.
  Bounded cleanup is tested with real processes, supervisor SIGKILL and locks.
- F5: zero-success baseline is rejected before admission collection/start.
- F6: one DB-anchored ninety-second application+native budget, checked on successful
  reads too; exact panel/count/summary parsing rejects1060 or contradictory results.
- F7: immutable fixed manifest precedes dispatch; fsynced started/finished/interrupted
  events retain partial work; final report is atomic and incomplete work cannot pass.
- F8: every result, class, flag and latency is validated; IPv4 is parsed before
  hashing; egress distinctness is derived from both valid hashes.
- F9: actionable Go evidence requires consistent outcome/HTTP/curl fields.
- F10: libpq parses protected connection data into0600 service/password files;
  neither DSN nor password is passed in argv. Read-only mode is verified.
- F11: zombie/dead process states cannot be reported active.

Additional serialization: proxy table SHARE prevents insert/enable races; selected
native plan and API identity are locked before freshness validation. Paired
freshness is checked again after assignment writes, before committing. Ambiguous
recording timestamps fail closed. The schema uses clock_timestamp after acquiring
the common performance lock; opposite transaction-begin order is explicitly tested.

A parent native hash may legitimately change if independent client membership
changes. This pilot deliberately fails closed on that drift, retaining
RECOVERY_PENDING instead of claiming exact restoration. The native planner test
proves exact before/candidate/restored hashes for stable membership and endpoint
identity. Fresh Output clients retain their authenticated route class; routing
selection lives in applied server settings, not in a generation parameter in URI.
Existing panel-config fencing orders in-flight candidate and restoration writes;
a stale candidate ACK cannot complete restoration. Relevant existing controller,
mutation-boundary and native-core tests remain part of the evidence bundle.

The CLI/Store/worker recovery integration tests use an isolated PostgreSQL schema.
Process faults and real Xray route tests are separate components, not a claim of
a complete production fault-injection run. Real field acceptance remains gated.


### Second review repairs

R1: accepted performance is provisional until unrejected automatic restoration
with reason "trial deadline expired" and the original retained deadline. Server
rejection, early cancellation or deadline drift revokes acceptance; exact recovery
can still be verified independently, including after restart.
R2 was contradicted by source and execution: time is used by earlierStability's
2*time.Minute; full-v3 (adminapi222.802s), vet-v3 and race-v3 passed. The import
was retained. Model findings are not authoritative when contradicted by evidence.
R3: supervisor bootstrap imports only the standard library. Freeze stages and
verifies the selected helper before use; recovery imports verified bytes from
staging and uses that same helper for database files and process supervision.
Fresh-process tests cover missing/replaced originals and corrupt staging.
R4: journal is created exclusively, fsynced and directory-fsynced before dispatch;
per-event fsync continues. Deterministic ordering and TERM/KILL tests pass.
R5: pre-mutation native/performance timestamps and lifetime are retained; final
DB-wall-clock validation checks them plus both named controls and receipt eligibility
after assignment writes. It does not demand APPLIED after deliberately marking PENDING.
R6: selected account and droplet share locks retain mutable lifecycle predicates.
Admission has existing3s lock/10s statement bounds. Account deletion generally
locks account then droplets; deletion completion can lock droplet before account.
That inverse path may deadlock or time out; PostgreSQL/Store abort the transaction
without partial admission, not bypass locks. Writer-before/after tests cover expiry,
delete and disable. Supported controller proof writes retain the performance lock.
R7: complete must explicitly be Boolean true; requested traffic phase must match.
Legacy recorded fixture outcomes are retained, with explicit completeness metadata.

Second-review regressions: Python46PASS and targeted isolated PostgreSQL PASS.
Final corrected-source full/race/vet and independent re-review remain release gates.

Final narrow N1 review PASS: a single clock sample defines the exact twenty-minute
freeze interval. Actual main --freeze regression passes with an advancing clock.
Python47, full-v4/vet-v4/race-v4, focused PostgreSQL and real native component
gates passed. Go graph is byte-identical across the final Python-only correction.
Release and fresh field qualification remain separate from source acceptance.
