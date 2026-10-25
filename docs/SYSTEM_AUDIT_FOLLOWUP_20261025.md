# System audit continuation — 2026-10-25

Latest API checkpoint: access restored; model metadata HTTP200; one real Astra/max
request timed out with no usage receipt. USD1.501011 stays reserved, with retries
blocked. See [OPENAI_ACCESS_RESTORED_20261025.md](OPENAI_ACCESS_RESTORED_20261025.md).

Starting point: source d8cc7206 and public stable 8d838efa. At 10:20 UTC (13:50 Tehran), all six services were healthy; default ran 8d838efa and production ran 5dc2c256. Final verification at 10:50 UTC (14:20 Tehran): public main/stable and default now run 80c85157; production remains healthy on 5dc2c256 because disk reserve still fails. All six executable hashes match their manifests and all ten readiness checks pass.

## Reproduced resource retention defect

Dispatcher.accounts retained zero-valued entries after each account's final handler finished. Recovery and lifecycle workers keep the dispatcher for the process lifetime, so distinct completed account identities accumulated without an active-work bound.

The baseline test completed 512 different accounts and found accounts=512, active=0, started=0. The fix removes an account only when its final handler actually returns. Cancellation alone does not free that account's reservation; completion of one sibling does not remove another sibling's reservation. All four focused dispatcher tests pass after the fix. Three new tests are mandatory in the unified 192-test publication gate.

The first local test invocation stopped before compilation because the transient service had no GOCACHE/HOME; it is recorded as a harness setup error, not a product failure. The corrected bounded invocation reproduced the product defect. Local tests use at most 512 MiB/one CPU and only dispatcher files; full package/race/database checks run on independent CI.

This fix bounds retained account identities. It does not establish a measured production memory reduction or explain the host's disk shortage.

## Reproduced publication-policy drift

The ops gate required 189 regressions while the standalone private-publisher copy
required only 146. After adding the three dispatcher tests they required 192 and
149 respectively. The private copy also lacked the pinned geosite asset and AWS
catalog browser assertion. The earlier checkpoint's 189-test CI result remains
valid for its ops path, but does not prove parity of the private publisher.

Two new parity tests failed against those divergent implementations. There is now
one self-contained implementation in tools/public-release; ops/release delegates
to it. The private publisher's credential-free one-file copy contract is preserved.
All ten Python gate tests pass, including standalone-copy evidence enforcement
and equal fail-closed CLI behavior. Source CI now runs these tests. Both entry
points enforce the same 192 regressions. The offline original host's installed
publisher has not been updated or claimed verified.

## Disk and deployment boundary

Read-only capacity inspection found about 27,871 MiB under a separate application's directory, including 26,929 MiB of data. That application's data/logs were not changed. The two backup vaults and retained database incident evidence were preserved. At the continuation preflight, 5,070,815,232 bytes were free against a production requirement of 6,547,416,125 bytes. This is an occupancy observation, not a time-series attribution of disk growth.

Production's previous verified stage /opt/.dob-release.bXqgHMgu remains intact. Do not bypass the reserve or start another production backup/deployment while its preflight cannot pass. No storage purchase, volume resize, unrelated application cleanup or database evidence deletion was performed.

## Review scope and stage state

| Stage | Current state |
| --- | --- |
| Prior checkpoint and live identity | Reverified |
| Production installation continuation | Blocked by disk reserve; prior services healthy |
| Worker resource retention | Reproduced and fixed; focused tests pass |
| Cancellation, sibling limits, admission | Relevant implementation inspected; focused behavioral tests pass; independent full CI passed |
| Publication policy drift | Reproduced; single implementation and 10 local gate tests pass; independent full CI passed |
| Provider interfaces and ownership | Shared registry/resource/absence contracts inspected; individual provider implementations remain pending |
| OpenAI review | Key still absent from known accessible configurations; 0 requests/tokens/USD this continuation |
| Publication | Source PR #5 merged; public main/stable 80c85157 verified and promoted |
| Default deployment | Completed under 1 GiB/one-CPU bound; three hashes, five readiness and 14 preservation checks pass |
| Broader load, restore and failure drills | Pending; no destructive live experiment |

SYSTEM_AUDIT_FOLLOWUP_20261025.json lists exact inspected files and hashes. No account settings, credentials, concurrency limits, provider actions or scheduling policies were changed. The original host remains quarantined and offline. No complete-project or future zero-failure guarantee is made.

## Network resource and account isolation review

The identity TLS cache bounds retained route scopes to 128 and tickets per scope
to eight, with an absolute ten-minute lifetime. The provider transport pool has
bounded entries, exact owner/credential/route revision partitioning and separate
HTTP-client wrappers per caller. Operational passive-health classification ignores
generic transport and cancellation errors; only explicit proxy authentication
failure provides negative passive evidence. The separate legacy
HealthReportingTransport helper is used in tests, not this operational path.
These findings are source inspection, supported by existing full-suite tests;
no live account stress result or complete provider-driver review is claimed.

The inventory now records 41 fully read files / 4,101 lines for this continuation,
with exact hashes. It does not count searches or partial file reads as full reviews.

## Independent verification and publication

- Source product head: e305d6dbf2adbb6eea74cb8cefaa917dabf37b9e;
  PR #5 merged as c309a6e60fa8507405c7b384dc98d23dff2a2fc4.
- Source CI 40185594319: full Go tests/vet, repeated mobile browser flows,
  all 192 required PostgreSQL/native/race tests with zero skips; 10 release-gate,
  25 bootstrap, 11 updater and eight fake-transport API-budget tests passed.
- Public exact candidate 80c8515752bbd9824967e2234c0c56ec789f55e8:
  CI 40185762641 passed source checks, fresh Ubuntu install and repeated install,
  all five readiness checks, ordinary login and identity preservation.
- Full export manifest (1,724 files), both live-secret scans and gitleaks passed.
- Promotion 40186013893 advanced main and stable atomically from 8d838efa to
  80c85157 with expected-head leases. Both remote refs were independently checked.

The production stage at /opt/.dob-release.bXqgHMgu is still the older 8d838efa
candidate; preserving it does not mean the new 80c85157 release is deployed.
The old resume helper is suitable for inspection of that older stage only; do not
call it a latest-release upgrade. After restoring adequate disk headroom, recheck
stable/CI identity and run the ordinary production updater under
ops/release/run-bounded-update.sh, preserving all backup/integrity guards.

## Final deployment state and next actions

The default installer completed successfully in a bounded service. Revision
80c85157, schema 000195_provider_snapshot_latest, all three running executable
hashes, five readiness checks and 14 policy/configuration/identity comparisons
passed. This independent instance currently has zero profile/Trojan identities;
those comparisons are not a production-fleet migration claim. Both instances'
six services were active with NRestarts=0 at the final snapshot. The installer
performed its normal controlled service restart; uninterrupted availability is
not claimed.

After the build finished and no Go compiler/linker or pending deployment remained,
only reproducible installer Go build/module caches were cleaned. Free space rose
from 4,594,343,936 to 4,934,873,088 bytes. The subsequent production preflight still
failed: 4,934,676,480 free versus 6,551,766,077 required. No production update,
additional production backup attempt, unrelated application cleanup or retained
database evidence deletion was performed in this continuation.

Next, restore production disk headroom including build/backup growth; reverify
the latest stable commit and independent CI before the bounded ordinary updater.
Do not mistake the preserved older 8d838efa stage for the new release. Continue
isolated restore/load/failure drills and per-provider semantic coverage; the
whole-project audit remains incomplete. The original host and installed private
publisher remain offline/quarantined. AWS credential acceptance, selected DNS
quota and gateway management remain unresolved.

The bounded Astra review helper retains its USD 2 / three-attempt cap. At the earlier 10:50 UTC checkpoint the key was inaccessible and no paid request
had been made. See OPENAI_ACCESS_RESTORED_20261025.md for the newer restored-access
checkpoint: one request timed out, actual usage is unknown and USD1.501011 remains
reserved; fake-transport tests are not API usage.
The legacy unbounded council was not invoked. Evidence is retained under
/root/dob-audit-followup-20261025. This checkpoint updates PROJECT_STATE.md and
HANDOFF.md so a later continuation uses repository state rather than chat memory.

## Credential access clarification — 2026-10-25T11:04:40.612795+00:00

The user confirms the API key remains on the original server at the previously recorded /etc/digital-ocean-bot/openai.env. Only its absence from accessible recovery-host configurations has been verified. Remote Desktop Commander is offline; the original IP responds to SSH, but available authentication is rejected. The correct blocker is access to the existing credential, not a claim that no key exists. Do not request the secret in chat. No API call or original-host mutation was performed during these checks.
