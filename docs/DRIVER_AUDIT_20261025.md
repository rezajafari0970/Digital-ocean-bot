# Provider driver audit continuation — 2026-10-25

Continues from source5703b6f5 and the restarted audit inventory. This round fully
read40 additional files (34 baseline Go files, four new Go files and two supporting
scripts). Cumulative coverage is88 files / 12421 current lines;
76 of1,218 baseline Go files / 10698 baseline lines.
The whole-project audit remains incomplete. Hashes and partial-read exclusions
are recorded in the cumulative and round-specific JSON inventories.

## Reproduced defects and correction

Vultr Regions/Plans/OS treated meta.links.next as a URL instead of the documented
opaque cursor. Existing tests only represented URLs without the production /v2
base, so they did not catch native cursor corruption or duplicated API prefixes.
Repeated pages had no loop/quantity guard; missing/null collections were accepted.
The controlled negative test made1,001 calls before its fixture stopped the old
loop. Cancellation also allowed another transport dispatch in the fake transport.

A provider-local catalog pager now fixes the endpoint and page size, encodes the
native cursor, extracts only the cursor from legacy URL-shaped values, rejects
ambiguous/missing URL cursors, stops cycles and caps1,000 pages, and checks
cancellation before another dispatch. Missing/null collections fail the whole
read. Empty explicit arrays remain valid. Mutations, account policy and instance
or SSH-key ownership paths are not changed by this catalog correction.
Official contract verified: https://www.vultr.com/api/ and
https://github.com/vultr/govultr/blob/master/README.md (Pagination).

DigitalOcean success JSON decoding and mutation error decoding were unbounded.
A16,777,230-byte synthetic body was consumed and accepted on GET/POST success;
the same oversized diagnostic was consumed on a503 mutation response. Null,
concatenated JSON and trailing garbage could also be accepted after the first
JSON value. A204/empty/null/missing-ID creation response was falsely accepted
with provider identity0.

Successful required JSON now has an8MiB plus one-byte detection bound and must
contain exactly one non-null JSON value. Error diagnostic reads are bounded
to32KiB while preserving HTTP status and Retry-After. Exactly-at-limit valid
JSON remains accepted. Droplet/SSH-key creation requires a positive provider
identity. Missing or unusable creation responses retain an ambiguous/retryable
transport outcome with no accepted identity; the client makes one mutation call
and does not resubmit. Normal no-content deletion remains supported.

## Verification stages

| Stage | Current evidence |
| --- | --- |
| Original/recovery identity | Source5703b6f5 clean; original services quarantined; six recovery services active, both readiness endpoints pass |
| Negative reproduction | Four Vultr and three DigitalOcean top-level regressions fail on original code; original logs retained privately |
| Focused correction | All63 top-level tests across both provider packages pass;10 publication-gate contract tests pass |
| Mandatory publication coverage | Eight added/extended regression roots required;201 required tests, including existing DB/native/race contracts |
| Independent source CI | Initial200-test candidate passed run40199667036; final201-test candidate requires a new run |
| Independent fresh/repeat installation | Pending |
| Stable publication and default deployment | Pending; live default still6c92d419 |
| Production deployment | Still disk-reserve blocked on5dc2c256 |
| API model review | No new generation request; previous unknown cost reservation remains blocked |

Negative and focused Go runs were bounded by systemd to1GiB/oneCPU; no live cloud
resource or database was mutated. Full Go/browser/vet and PostgreSQL/race checks
belong on the independent CI runner. Successful fixture tests do not establish
live provider acceptance or production-fleet rollout.

## Additional inspection findings and remaining scope

All six provider HTTP entry implementations were read, together with their shared
credential boundary and selected transport isolation code. Linode/AWS/OVH/UpCloud
already bound individual response bodies. Their full mutation/lifecycle/panel
implementations and long-run aggregate-resource behavior are not all reviewed.
Vultr instance, SSH and reserved-IP pagination have separate existing guards;
their semantic/identity acceptance must remain independent of catalog recovery.

Dynamic plugin loading, complete exported metrics/tracing, distributed fencing,
isolated restore/fault/load drills and the prior recovery-discovery timeout cause
remain unfinished work. Original database/storage quarantine, production disk
headroom and provider usage reconciliation remain operational blockers.
Never restart the original cluster or bypass the reserve/API ledger.

Private reproduction evidence:
 /root/dob-driver-audit-20261025/negative.json and negative.log,
 negative-response.json and negative-response.log, focused.json and focused.log.
The baseline failing response fixture emitted a large synthetic error line;
that evidence is retained, and subsequent test diagnostics are bounded.


## Central creation outcome ordering

The full executor and reconciliation path was then read. An explicit ambiguous
result with no returned error and no resource ID was checked as missing-ID failure
before the ambiguous-outcome branch. A new subcase of the existing executor test
reproduced state=failed. The branch order is corrected: both known-ID and no-ID
ambiguous results stay UNKNOWN and resumption observes ownership without a second
create. Rejected or empty accepted outcomes retain their existing validation.
This extends the required gate to201 tests and requires a fresh exact-source CI.

The deployment SSH-key path has a separate unresolved durability concern:
it currently saves the newly generated private key/snapshot after provider
import. An uncertain import or crash can therefore lose the matching local
identity. No live occurrence is claimed and this release does not redesign that
flow. Its next review must establish durable key/intent reservation before
external import, scoped reconciliation and crash/concurrency tests. The current
driver's single-call guarantee is not a proof of end-to-end SSH-key exactly-once
behavior.
