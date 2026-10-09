# Bounded residential stability pilot (draft, 2026-10-09)

**Release status: NOT DEPLOYED / independent review blocked by API credit.**
This document describes the implemented candidate, not accepted production behavior.
Baseline: 94b1f4dfbba3d3d71632fe276c61a5994f53ed10.
Current production / rollback source: fc76e6a669931873295ddded4f51eca3a7bbe8ca.
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

Candidate native generation must be proved within ninety seconds. Complete native
and actual traffic proof must finish before the original five-minute deadline,
with owned tuning/generation checks around measurements. Failure requests owned
cancellation. Acceptance waits for automatic deadline restoration.

COMPLETE means exact parent restoration, generation base+2, fresh APPLIED/native
proof and a successful final route check. trial_outcome is independently accepted
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
   paths with exactly admission, tune, native, traffic and xray keys.
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

Do not run the candidate against production while its independent review is blocked.
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
