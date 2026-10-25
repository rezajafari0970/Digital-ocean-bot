# System audit continuation — 2026-10-25

This continuation starts from source d8cc7206 and public stable 8d838efa. All six services were ready with zero automatic restarts at 10:20 UTC (13:50 Tehran). Default still runs 8d838efa; production still runs 5dc2c256.

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
| Cancellation, sibling limits, admission | Relevant implementation inspected; focused behavioral tests pass; full CI pending |
| Publication policy drift | Reproduced; single implementation and 10 local gate tests pass; full CI pending |
| Provider interfaces and ownership | Shared registry/resource/absence contracts inspected; individual provider implementations remain pending |
| OpenAI review | Key still absent from known accessible configurations; 0 requests/tokens/USD this continuation |
| Publication and deployment | Pending independent verification |
| Broader load, restore and failure drills | Pending; no destructive live experiment |

SYSTEM_AUDIT_FOLLOWUP_20261025.json lists exact inspected files and hashes. No account settings, credentials, concurrency limits, provider actions or scheduling policies were changed. The original host remains quarantined and offline. No complete-project or future zero-failure guarantee is made.
