# Lifecycle acceptance follow-up — 2026-10-12

Recorded at 2026-10-12T04:33:52.027665+03:30. Runtime remains `9e59e3a45f90e6b98043c2bae66b88b8cad352f8`; tested source is `c19c7d0ad258235430e83ecaf69e8b2ceff5392e`. This follow-up changes documentation only. No new provider account, cloud server, account deletion, production database mutation, service restart or policy change was initiated by this acceptance run. Existing workers continued their already-authorized lifecycle.

## Verdict by requested stage

| Stage | Verified result | Remaining condition |
| --- | --- | --- |
| 1. Capacity and the two earlier deletions | PASS for capacity: last observed 225/225, zero deficit. Both repaired Vultr panels now have five clients each. | The two specifically tracked deletions are not complete. They are queued behind the configured account backfill/spacing policy. |
| 2. Cloud resources of removed accounts | Local account absence and empty account-purge queue verified. | BLOCKED for cloud deletion proof: Sara vul and Sara vul 2 were locally removed after invalid-credential evidence. Last historical inventory was 16+15 resources; current cloud count is unknown. |
| 3. End-to-end acceptance | PASS for isolated contracts: 68 top-level tests, 153 including subtests, zero failures and zero unresolved skips. Actual post-release production deletion and output removal also verified separately. | A complete dedicated live-cloud-account lifecycle was not run because no dedicated test account/credentials were available. Fixture drivers are not claimed as cloud-provider proof. |
| 4. Record evidence and API usage | PASS for evidence collection and report generation. | Overall cloud acceptance cannot be marked PASS while the access-dependent conditions above remain unresolved. |

## Current production observation

Observed 2026-10-12T04:30:55.808485+03:30: 45 active servers, zero inactive/degraded/broken/provider-stale servers; 4 in deletion/expiry lifecycle, 49 total. All 45 serving routes have fresh native proof, maximum proof age 55 seconds, zero route errors. Account-purge queue is empty. All three process hashes match the manifest, readiness is 5/5, automatic restart counters are zero.

Capacity is an explicit last-observed census: 45 eligible and observed inbounds, 30 within the separate 30-second observation window; target225, active225, deficit0. This is not a claim that all45 were synchronously read within30seconds. Performance remains RESTORED. Configuration/environment/systemd hashes equal the previously accepted release baseline.

## Why the two earlier servers have not disappeared

At04:26:51 Tehran, Vultr Usman1 had9 managed servers against Desired10, no pending deployment, no recorded lifecycle failure or recovery checkpoint. Configured Build spacing is63..123minutes; next_build_at was05:24:15.177084 Tehran. Source `internal/app/lifecycle.go` deliberately waits for scheduler backfill below Desired. `internal/droplets/lifecycle.go` admits one item per account, prioritizing EXPIRING before other expired READY rows. Thus the earliest EXPIRING row waits for backfill and a later expired READY row remains behind it. This is recorded policy behavior, not completed deletion or a provider DELETE failure. The next-build timestamp is the earliest scheduled opportunity, not a promised deletion-completion time.

Tracked local IDs: `c765f068-3a4d-4501-928f-d79d781aedf0` (EXPIRING) and `9d2d7ad9-00e0-4fc0-ab06-c3f28c2fe0ff` (expired READY). Their provider DELETE operations had not been admitted at diagnosis. No forced status, queue reset, permission bypass, lifetime change or Build spacing reduction was used to obtain a passing result.

## Distinct real production deletion evidence

Provider resource608216312 on Oceano new1 has DELETE_DROPLET succeeded, attempt1, completion00:38:25 UTC after the accepted code release. The local droplet is DELETED, owned resource is deleted, panel is disabled and its output_config_snapshots count is0. This confirms this production delete path; it does not certify deletion of the two waiting Vultr resources or resources belonging to removed Sara accounts.

## Isolated validation scope

Fresh race-enabled tests exercised creation workflow stages, resume without duplicate creation, PostgreSQL lifecycle discovery, capacity/expiry guards, deletion confirmation and unknown-outcome recovery, client lifecycle/replacement, output expiry, account dependency purge and actual Chromium desktop/mobile account deletion/queued purge flows. All database connections used `dob_bulk_test_lifecycle_20261012` and fake provider/native fixture endpoints. No production admin session was manufactured.

The first run explicitly skipped `TestRetiringWithoutExpiryIsDueE2E` because its dedicated DSN was unset. The isolated database was then initialized with the project migration runner and that exact test passed with its dedicated test DSN. The skip remains in raw history and is resolved by the second receipt; no skipped test is counted as a pass without execution.

Raw evidence: `/root/backups/dob-lifecycle-acceptance-20261012`: stage1 identities/diagnosis, real-deletion/output evidence, test JSONL and receipts, test-summary.json, final native/bootstrap/metrics/readiness captures, configuration comparison and acceptance.json. No blanket claim of a single real-cloud workflow spanning all mocked and observed stages is made.

## OpenAI API usage and unblock requirements

This follow-up: 0 external OpenAI generation requests, 0 tokens. Existing test and read-only operational evidence did not require another model review; production source was unchanged. Prior panel audit remains11requests/197126tokens. Including the separate earlier account-cleanup task:15requests/372886tokens. Historical monetary cost is unavailable because receipts report no charge. These counts do not measure ChatGPT Work billing.

Cloud closure requires valid authorized provider access for the same two removed accounts, or provider-side deletion records verifiable against their resources. Do not recover deleted credentials from backups, restore accounts, infer cloud deletion from local absence or bypass the account proxy/authorization path. A dedicated live-cloud acceptance needs a separately identified authorized test account. Until then the cloud portions remain BLOCKED/NOT RUN, not PASS. Historical backups and provider billing records remain outside operational purge.
