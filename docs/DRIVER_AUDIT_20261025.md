# Provider driver audit continuation — 2026-10-25

Continues from source 5703b6f5 and the restarted audit inventory. This round fully
read 40 additional files (34 baseline Go files, four new Go files and two supporting
scripts). Cumulative coverage is 88 files / 12421 current lines;
76 of 1,218 baseline Go files / 10698 baseline lines.
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
A 16,777,230-byte synthetic body was consumed and accepted on GET/POST success;
the same oversized diagnostic was consumed on a 503 mutation response. Null,
concatenated JSON and trailing garbage could also be accepted after the first
JSON value. A 204/empty/null/missing-ID creation response was falsely accepted
with provider identity0.

Successful required JSON now has an8MiB plus one-byte detection bound and must
contain exactly one non-null JSON value. Error diagnostic reads are bounded
to 32KiB while preserving HTTP status and Retry-After. Exactly-at-limit valid
JSON remains accepted. Droplet/SSH-key creation requires a positive provider
identity. Missing or unusable creation responses retain an ambiguous/retryable
transport outcome with no accepted identity; the client makes one mutation call
and does not resubmit. Normal no-content deletion remains supported.

## Verification stages

| Stage | Current evidence |
| --- | --- |
| Original/recovery identity | Canonical merge5ae2fd7c; original quarantine retained; all six recovery hashes and both readiness endpoints pass |
| Negative reproduction | Four Vultr and three DigitalOcean top-level regressions fail on original code; original logs retained privately |
| Focused correction | Initial 63 provider tests pass; final 86 focused tests pass with 9 database/E2E-only skips recorded;10 gate contract tests pass |
| Mandatory publication coverage | Eight added/extended regression roots required;201 required tests, including existing DB/native/race contracts |
| Independent source CI | Final run 40200418124 passed full Go/vet/browser,201 mandatory DB/native/race tests with zero required skips, and 20 isolation repetitions |
| Independent fresh/repeat installation | Exact public run 40200848276 passed Ubuntu24.04 amd64 install/reinstall, five readiness checks, login and identity preservation |
| Stable publication and default deployment | Atomic promotion40201086442 passed; main/stable/default now a59597c4, all 14 preservation checks pass |
| Production deployment | Still disk-reserve blocked on 5dc2c256 |
| API model review | No new generation request; previous unknown cost reservation remains blocked |

Negative and focused Go runs were bounded by systemd to1GiB/one CPU; no live cloud
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
This extends the required gate to 201 tests; final exact-source CI 40200418124 passed.

The deployment SSH-key path has a separate unresolved durability concern:
it currently saves the newly generated private key/snapshot after provider
import. An uncertain import or crash can therefore lose the matching local
identity. No live occurrence is claimed and this release does not redesign that
flow. Its next review must establish durable key/intent reservation before
external import, scoped reconciliation and crash/concurrency tests. The current
driver's single-call guarantee is not a proof of end-to-end SSH-key exactly-once
behavior.


## Final release and live acceptance

Implementation 640935c5704c3d195850146b139a56a4aab7f7cd merged through PR #8 as
5ae2fd7cc410592f9954fe9ba3048b0c301c43ae. The full 1,729-file export passed BOTH
live-configuration secret comparisons and gitleaks. Initial CI 40199667036 passed
the earlier 200-test candidate; final CI 40200418124 independently passed 201
required tests with zero skips, full source/vet/browser checks and 20 unchanged
recovery/lifecycle isolation repetitions. The9 focused local DB/E2E skips are
preserved as skips, not relabeled as passes or as complete DB-suite coverage.

Public a59597c4f6b1f811269818e71087b3f502f260ee passed independent source and exact
fresh/repeated Ubuntu24.04 amd64 installation in40200848276. Both install receipts
verify five readiness checks, ordinary login and preserved installation identity.
Promotion40201086442 moved main/stable atomically with expected-head leases.
Both actual remote refs were independently verified.

The default installation was updated from 6c92d419 through the ordinary guarded
installer under 1GiB/one CPU, completing in about 235 seconds. A private configuration
baseline and fresh dump preceded the update. A controlled restart occurred;
this is not a zero-downtime assertion. Schema remains 195. All 14 configuration,
policy and ownership checks pass; all three executing hashes match the installed
manifest. Default has zero profile/Trojan identities, so this does not prove
production-fleet acceptance.

At 17:48 Tehran (14:18UTC), both recovery installations passed all five readiness
checks and all six executing hashes, with NRestarts=0. Default runs a59597c4;
production remains5dc2c256. Original PostgreSQL/API/workers/publisher were verified
inactive at 17:44 Tehran; original quarantine and disabled publisher are retained.

The normal guarded updater removed one expired verified runtime copy and one
expired verified backup through its existing retention policy. After it exited,
only completed reproducible Go caches were manually removed. No incident
evidence or unrelated application data was deleted. The final read-only reserve
check remains blocked:4,123,324,416 bytes free versus6,687,351,869 required.
Its retained candidate is OLD 8d838efa, not current a59597c4. Restore headroom for
build/backup growth, reverify current stable/CI and use the ordinary bounded
production updater with fresh integrity/archive guards; do not bypass reserves
or treat the old fixed-stage resume helper as a latest-release deployment.

No new paid model generation was submitted. Original ledger schema 2 still has
one unknown attempt andUSD 1.501011 reserved underUSD 2/three attempts. The last
usage reconciliation returnedHTTP403; actual tokens and billed cost remain
unknown. No reset, override, replacement ledger or blind retry was used.

## Exact next work

First address the separately documented deployment SSH-key durability boundary:
persist encrypted key and an account/deployment-bound intent before provider
import; resume the same identity after restart; reconcile ambiguous imports using
validated ownership/fingerprint; reject conflicting or stale owners. Test crashes
before and after dispatch, lost responses, concurrent admission and credential
generation changes without creating live cloud resources. Do not substitute an
automatic retry or silently change operator policies.

Then continue the pending provider/panel/worker inventory and isolated restore,
fault, sustained-load and multi-host ownership work. Full exported metrics and
tracing remain incomplete. Twenty successful recovery repetitions do not prove
the earlier discovery-timeout cause is fixed. The project is not certified
permanently error-free or fully audited.

Evidence:
- https://github.com/rezajafari0970/Digital-ocean-bot/pull/8
- https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40200418124
- https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40200848276
- https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40201086442

Aggregate acceptance: docs/DRIVER_AUDIT_20261025_ACCEPTANCE.json.
Private logs, baseline/dump and preservation evidence:
 /root/dob-driver-audit-20261025.
