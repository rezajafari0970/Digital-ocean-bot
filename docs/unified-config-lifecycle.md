# Unified configuration lifecycle

Every configuration is a profile with its own protocol, network, security, port, server scope, routing classes, client count and desired revision. No protocol has a privileged creation path. The existing driver registry composes independently registered protocol, transport and security implementations; only combinations tested against the pinned official core are advertised publicly.

## User workflow

Create with New, select a supported combination, choose an explicit port or leave it empty for automatic reservation, set the count and scope, then Save. Saving schedules reconciliation automatically. Edit changes the same profile. Pause removes its active listeners and output while retaining identities for Resume. Delete permanently retires its users and listeners; the port is released only after confirmed absence. No manual client-maintenance or transport-apply step is needed.

The count is per routing class per server. Direct and Residential remain separate immutable client identities. Count increases retain existing users; decreases remove surplus native and global users through a durable retirement journal. Profiles follow server lifetime. Public Trojan uses TLS on the supported official core; plaintext Trojan is not advertised as a working public capability.

## Ownership and recovery

- Desired state, applied native state, runtime verification and published output are separate recorded stages.
- Every profile/server pair has an advisory lease. Panel mutations use the existing panel lock and serialized admission gate.
- Atomic due-row claims prioritize the earliest due work, then the oldest attempt, distributing retries fairly across profiles and servers. Status and enrollment use the same eligibility predicate, including servers near the end of their lifetime.
- Prepared and sent mutations freeze the exact specification, membership and credential reference. Lost responses trigger fresh observation before any further mutation.
- Native confirmation is persisted before connection proof. A slow connection test does not repeat creation or monopolize the creation lanes.
- Updates and deletion verify the owned native identity and a hash of sibling listeners. Foreign changes block only the affected operation.
- Port admission reserves desired, applied and uncertain pending ports. Native absence plus running-core socket absence is required before releasing an applied reservation.
- Removed global users remain journaled until their exact identities are observed absent. Restarting a worker does not forget unfinished cleanup.
- SQL admission accounts for both profile sessions and independent routing-proof reservations. With a 40-connection panel pool and 18 serving proofs, ordinary panel work is capped at eight operations; nested queries and heartbeat publication retain headroom.
- Failures have bounded retries and recorded stages. Runtime failure withholds that profile's output until verification succeeds.

## Statistics and interface recovery

Ready-server and published-config counts use the same publication predicate as subscriptions, in one repeatable-read database snapshot. Native verification alone does not imply a published connection. Old revisions, stale output, failed routing proof, disabled accounts and incomplete cleanup are excluded.

The browser preserves the last observed cards with an explicit stale notice when reads fail. Reads have a hard deadline, a single in-flight request per mounted page, and page-identity checks. Navigation, session expiration or late responses cannot overwrite a newer page. Controls wait for the saved revision before another action on that profile.

## Upgrade and retirement

Migration 185 adds journals and authority state without deleting existing native listeners. An existing installation activates the unified authority through the authenticated `POST /api/v1/configs/activate-unified` operation after backup and deployment verification. It records one durable cleanup job, retires previous configuration profiles and disables legacy Reality, Trojan, bulk and manual creation authorities. Database triggers prevent those legacy policies from being re-enabled.

Cleanup proceeds independently per server and retries recoverable failures. A completed server becomes eligible immediately; it does not wait for unrelated failed servers. New profile intents can be saved during cleanup. A fresh installation with no existing panels activates the single authority automatically and creates no default protocol.

## Verification evidence

The required disposable-PostgreSQL gate includes profile resize, preserved identities, pause/resume, lost mutation responses, sibling preservation, confirmed deletion, port reuse, idempotent creation, migration retention, exact publication counts, cleanup isolation and legacy-authority retirement. Native integration exercises all 16 publicly supported combinations through the pinned panel and official Xray, including authenticated traffic. Mobile Chromium tests cover the unified workflow, transport-specific paths, stale edits, failed and hung reads, navigation, authentication and layout.

External provider, network and certificate failures remain possible. The controller reports them and retries without claiming successful output or changing unrelated profiles.


## Independent class controls — 2026-10-18 continuation

Schema 186 stores Direct and Residential count, lifetime (seconds; UI minutes/hours), device limit, quota, creation rate/cadence, Flow, publish, TLS allowInsecure and client FinalMask/export independently. Existing credentials retain identity. Expired clients are retired and replenished by their own policy. Each class requires its own authenticated traffic proof before publication. The old global output formatter cannot erase a profile's chosen mask. Profiles continue to reserve independent ports and native ownership.

Reality SNI selection is profile/server/revision-scoped, validates public TLS 1.3/h2/certificate results, caches six hours and retries only the affected profile. Repeated automatic-target failures trigger re-selection; manual names never silently change. Public Trojan without TLS remains unsupported by the official Xray client: advertising it as a working public configuration would be incorrect.

Runtime verification batches a panel's profiles through one SSH connection, with separate membership/traffic outcomes and bounded four-profile concurrency. Native mutation remains fenced per panel; proof-only readers can observe after the final write. Durable leases and next-attempt/stage timestamps prevent abandoned workers from claiming permanent ownership. Counts come from one repeatable-read snapshot and show per-class created and published totals; zero publication does not imply an uncreated listener is ready.

Live diagnosis found that 13 inbounds require 429 routing matrix entries. The old 256 cap forced remote HTTP fallback until timeout. The revised local batch accepts a bounded 4096 entries, keeps every indexed response, and compresses receipts to fit the existing SSH output bound. A real 429/429 read-only check completed in 8.06 seconds. Each panel remains sequential; distributed serving proofs have 18 bounded slots.

Validation before rollout: all Go tests and vet passed; mobile Chromium verified independent lifetime units/count/device/publication, stale response recovery and deletion isolation; 37 required isolated PostgreSQL/race/native regressions passed with no skips. The job JSON records deployment separately from source validation. Production is not considered updated until exact installed manifests and canaries pass.


## Native device policies and bounded proof waves — 2026-10-18

Release a11eb583 (source 5f799e4, schema 186) was installed and verified on both hosts. The production route-batch correction restored publication; later inventory contained 990 VERIFIED listeners plus nine with a partial class proof. These are time-bound observations, not a claim of complete current coverage.

A real canary exposed a native v3 behavior absent from the old mock: inbound writes ignore `limitHwid`; the global client record owns this policy. The controller now confirms exact UUID/email ownership, preserves native update fields, applies expiry/quota/device values to that record and requires fresh readback. A lost SET acknowledgement is handled by observation. Unrelated global records cannot block the owned policy. The lifecycle fixture now models this behavior and asserts actual global values.

Native confirmation is a separate persisted phase (`routes_pending`). The writer releases the panel before route/runtime tests; the proof lane completes publication. A busy panel only reschedules the operation and does not erase healthy state. Runtime batches use one parallel wave of four profiles, avoiding several timeout waves exhausting the 45-second worker deadline.

Read-only native proof at 06:57:42 UTC confirmed the recorded Trojan canary: Direct = one client, unlimited lifetime, device limit 3; Residential = one client, 7200 seconds, device limit 4. All 37 mandatory isolated database/race/native regressions passed with zero skips; policy unit tests passed. Deployment and the full canary expiry/edit/delete cycle remain pending for this hotfix. Existing user profiles are preserved; previous legacy retirement must not be repeated.

Server-detail refresh also runs when aggregate counts are unchanged. A progressing stage or retry can no longer remain visually frozen solely because the profile totals have not changed. Native-policy, routing and queued-operation stages have explicit user-facing labels.


## Admission deadline and first-publication follow-up

Release 77068b5 was installed and verified on both hosts (07:11:46 and 07:14:17 UTC), preserving all user profiles and five readiness samples each. Live follow-up identified that the verification deadline began before admission to the shared work budget. Queued operations therefore could begin SSH with too little time. Execution deadlines now begin after admission; cancellation still follows worker shutdown. First publication and applied native changes are scheduled ahead of routine refreshes without privileging a protocol. Cancellation during a proof read leaves prior positive evidence governed by the unchanged five-minute freshness fence instead of inventing a negative connection result.

The isolated lifecycle test now checks cancellation preservation. Separate worker tests cover queue wait beyond the operation duration and cancelled admission. All 37 mandatory regressions passed. An earlier VLESS/WS/TLS native loopback run returned HTTP 503 intermittently; its failure log is retained, and no gate or native assertion was relaxed.

The canary SNI scanner selected www.microsoft.com after native checks (3 ms latency). All three canaries reached publication at 07:19:50. VLESS native Vision is intentionally exported as the documented client Vision-udp443 variant; the canary checker now uses that contract. Expiry/edit/output/deletion validation continues.


## 2026-10-18 07:54 UTC — native transport repair and queue continuation

Installed release remains 77068b527a05d7b79abbe08756f84c547a9fc116 on both controllers. Independent class policies and native device-limit readback are deployed. Queue-admission correction and the following core repair are pending product publication and staged installation.

The publisher correctly blocked the prior candidate after a reproducible VLESS WebSocket TLS HTTP 503. Native WebSocket dropped n>0 bytes returned with frame EOF and propagated normal close 1000 as a transport failure. Fixes preserve final bytes and normal stream completion; abnormal close codes remain errors. Race validation then exposed certificate reload/OCSP mutating shared TLS storage. Immutable atomic certificate snapshots fix concurrent reload while retaining prior handshake certificates and configuration.

Native artifact 3dbe70e8a900cb540c7c6e38aea5e78027dfed57 (dob-fm-0.2.3) includes complete source, patches, licenses and validation hashes. Final binaries passed three actual 16-public-variant roundtrip runs, the loopback-only Trojan none fixture, and three TLS/WebSocket race suites. Earlier focused WebSocket repair passed 30 consecutive native tests. No failed gate was relaxed.

Unified reconciliation now prepares the pinned core under the existing per-panel writer lease, regardless of profile protocol. Native proof identifies an older recognized core and queues exactly one replacement per panel; previous positive traffic proof remains subject to its unchanged freshness fence. Unknown binaries never authorize output. Installation validates the actual configuration, journals identities/backups and supports rollback. Cancelling a controller worker does not invent negative remote evidence. Existing user profiles and credentials remain protected. Live expiry/edit/deletion acceptance and final fleet coverage are still pending.


## 2026-10-18 follow-up: output, independent queues and observed counts

The phone recording showed created listeners losing publication coverage while the viewer retried. The viewer rejected valid VMess/SS lines; it now accepts all four registered URI schemes without mixing status text into copyable output. The server's Trojan output boundary also accepts explicit TCP/none. Plain public VLESS/Trojan server profiles are available with explicit compatibility metadata: recent official Xray clients reject those public unencrypted outbounds; server-side generation cannot override that client restriction.

A v3 global client can retain Flow after an inbound update. Recovery observes the exact owned membership and all other limits, applies the explicit global Flow SET, then requires a fresh strict read before confirming. The same reconciliation repairs managed Flow drift against the prior confirmed credential generation before replacing expired users. No mismatched identity is adopted and no blind inbound POST is replayed.

Creation, deletion and proof work now have separate bounded classes (four creation workers, two deletion workers, eight proof workers) under the existing SQL admission ceiling. Verification batches up to eight profiles in two waves of four. One immutable runtime/process/socket observation is shared within the held panel configuration fence; streaming binary hashing replaces repeated whole-binary allocations. The duplicate proof advisory-lock session is removed; the exclusive configuration fence remains. Proof work has a 75-second admitted deadline and a 90-second supervisory ceiling. Five-minute runtime proof freshness and one-minute routing freshness are unchanged.

Deletion uses an exact profile/revision snapshot. Stale edits conflict individually, retries reuse durable per-profile tombstones, and later creations are excluded. Ports remain reserved until native absence is confirmed. The UI asks for confirmation before submitting the snapshot; merely installing the button removes nothing. Open server details poll independently every two seconds, coalesce overlapping reads, and retain their DOM while aggregate counts change.

Migration 187 separates ownership reservations from observed native clients. Confirmation and native proof mark observed members; pending reservations cannot inflate active counts or enter publication. Publication statistics evaluate their expensive eligibility fences once per repeatable-read snapshot rather than repeatedly per profile and class.

Validation and live acceptance are recorded in the accompanying development-job JSON. A single-panel sample of eight profiles / sixteen class probes passed before and after runtime observation changes (10.79 seconds and 9.69 seconds respectively); this is not a fleet-wide speed guarantee. Both previously rolled-back native upgrades succeeded after an exact-journal, unchanged-identity review, retaining the original rollback records. Future rollbacks preserve their failed action and sanitized cause. Current user profiles were not bulk-deleted.

### Live acceptance follow-up: verification deadlines and native deletion

Release acb6a001 was installed and preservation-verified on both hosts (schema 187). Catalog response dropped from a 3.14-second sample to 0.29–0.57 seconds; complete fleet publication remained below target and is not claimed complete. Direct-only edits passed live output checks while retaining all three Residential client identities. Native lifetime/device/Flow readback also passed.

The live batch-delete acceptance exposed a missing policy argument in the deletion observer: Sanaei v3 stores HWID caps in its global client table, while inbound inventory omits them. Deletion now uses the same strict owned-policy observation as creation; exact identity, membership, transport, sibling preservation and global-user absence checks remain required. The mandatory lifecycle regression now includes nonzero independent device/lifetime policies and uncertain deletion acknowledgement. Busy deletion and proof workers requeue in two seconds without recording false failure.

Due proof claims now prioritize the oldest verification timestamp, preventing repeated busy claims from moving stale proof behind routine healthy refreshes. A batch admits up to 16 profiles with the same four concurrent native clients; its operation deadline is 120 seconds and supervision deadline 150 seconds. The live 16-profile sample completed 31 successful class probes in 16.86 seconds. The five-minute publication fence and TLS verification are unchanged. This follow-up requires publication, staged rollout and another coverage observation.

## 2026-10-18 10:00 UTC — live acceptance and bulk deletion follow-up

Release 2de6fd9 (source 4737cc3, schema187) is installed and exact-binary/readiness/preservation verified on both hosts. The native runtime audit passed all 81 live panels. All four temporary test profiles have verified native listener/global-user absence. The final automatic VMess cycle produced two Direct and one Residential output in 64.419 seconds and removed them in 32.160 seconds, with independent limits confirmed. No manual reconciliation was used for that cycle.

At 09:41:21.795273 UTC a separate authenticated batch request marked all fourteen user profiles for deletion. This task did not submit that request; the prior endpoint did not record the actor, so no requester is inferred. Empty output afterward is expected. Deletion is progressing but only two deletion workers limit fleet throughput. Eight deletion workers now share the unchanged SQL-derived admission ceiling, retaining per-panel write exclusion and native-absence confirmation before port reuse. Transactional audit records the authenticated actor, exact profile/revision snapshot, conflicts and newly created intents, including idempotent retries.

The 13:15 phone recording exposes a status defect: deletion increments the desired revision, which made server details incorrectly say Applying saved changes. API details now carry explicit removal intent; the view distinguishes queued removal, actual deletion and confirmed absence, ignoring obsolete connection errors during deletion. Output keeps valid freshly fetched protocol entries if another line is malformed, reports the omitted entries separately, and still clears credentials on failed requests, hidden pages and authoritative empty responses.

Before the bulk deletion, the four-minute observation had 975 fresh proofs out of 1134 eligible listeners and 1864 exported links. Complete sustained fleet coverage was not established; zero eligible profiles after deletion must not be reported as 100% coverage. No 99% speedup or permanent absence of faults is claimed.

## 2026-10-18 continuation: bounded creation capacity

Bulk release 033bf2c (source dd0f4c3) is installed on both hosts with exact live binary hashes, five readiness samples and preserved policies/credentials. The previous fourteen-profile removal completed: zero native references remain and 1101 listeners carry confirmed deletion. Three subsequently created user profiles are preserved.

Fleet creation still had only four lanes while deletion/proof had eight. Creation now has eight independently supervised lanes under the same SQL-derived shared admission ceiling; this does not increase the total admitted long-running work or permit concurrent writers on one panel. Worker/work-budget race suites passed. Publication gates and live acceptance remain required; no fleet-wide speed percentage is asserted.

## 2026-10-18 continuation: transient traffic verification

Creation-capacity release 75ba684f (source e6a866e) passed all 37 required regressions and was installed and preservation-verified on both hosts. A fully automatic isolated cycle published two Direct and one Residential client in 37.519 seconds and confirmed removal in 7.284 seconds; fresh native reads confirmed exact limits and then listener/global-user absence. Three user profiles were preserved. These samples do not establish a fleet-wide percentage speedup.

The five-minute observation exposed intermittent single-class proof failures despite all native listeners being created. Read-only native diagnostics of four affected samples shortly afterward returned HTTP 200 for both classes. A shared HTTPS helper now retries a transport failure or selected transient 5xx once over a fresh connection. Positive TLS/HTTP/body evidence remains mandatory; invalid certificates, 403, 429, empty success bodies and persistent errors stay rejected. Each connection closes and the existing admitted task deadline remains authoritative. Regression tests cover these boundaries. Deployment and post-release coverage are recorded separately.

## 2026-10-18 final continuation acceptance

Final public release `35aa1fd203826810ba68821b8873a2446d4d9d83` (code source `93808b0fc95946d162db38e6df4495f2cc3b6bea`, schema187) is installed on both controllers. Secondary verification completed at 11:07:27 UTC and primary at 11:10:44 UTC. Exact live binary hashes, all three services, zero restarts, five readiness samples and policy/credential/native-identity preservation passed on each host. The publisher passed all 37 mandatory database/race/native regressions with zero skips, full Go tests and vet, six Output viewer cases and the sandbox installer. The shared strict-TLS transient retry also passed targeted race tests and read-only native positive proofs on two real servers.

Creation, deletion and proof each have eight class lanes beneath the unchanged SQL-derived parent budget; they do not admit 24 concurrent writers. Per-panel write exclusion, ownership fences, route freshness and TLS verification remain enforced. Removal status and actor/snapshot audit are deployed. A malformed output entry no longer discards valid independent entries.

The previous fourteen-profile deletion is complete: 1101 confirmed removals and zero remaining native references, with no running deletion or retry error at 2026-10-18T11:11:39.858721+00:00. Both continuation canaries have confirmed listener/global-user absence. The three subsequent user profiles remain. The automatic live cycle measured 37.519 seconds to output and 7.284 seconds to confirmed absence with exact independent client policies; no manual reconcile was used.

Post-release observation ran from 2026-10-18T11:11:02.460894+00:00 through 2026-10-18T11:14:03.994076+00:00 (181.533 seconds; 13 samples). All readiness and Output HTTP requests passed. Publication coverage ranged 98.78–100.0%; 9/13 samples had every eligible listener ready. The four incomplete samples occurred after a newly eligible panel joined; its three profiles were still being created with no blocked state. A separate six-sample follow-up retained the initial observation and checked automatic completion. The final follow-up sample at 2026-10-18T11:15:56.726963+00:00 had 492/492 published links, 492 actual output lines, and no pending or blocked profiles. Across all 19 observations, Output latency was 39–67 ms (median 48 ms); minimum available memory was 4319 MiB. Changes in eligible fleet size follow server lifecycle and must not be confused with output faults.

Nineteen closed backup archives were losslessly compressed, saving 3,554,673,865 bytes; decompressed SHA256 and exact archive members/metadata were verified. No backup was deleted and no install reserve gate was bypassed. No external OpenAI API calls were made in this continuation.

All identified implementation and deployment steps in this continuation are complete. These live samples establish observed behavior during the stated interval, not a 99% fleet-wide speedup or a guarantee against future provider/network failures. The documented recent official Xray client restriction on public unencrypted VLESS/Trojan remains an external compatibility constraint.

## 2026-10-18 15:13 Tehran continuation: fresh status check

Both hosts still run public release 35aa1fd with all readiness checks passing, three active services each and zero restarts. No product code changed and prior test results were not presented as newly rerun tests. The source and acceptance checkpoint were synchronized at 0a2093b before this recheck.

Initial observations included 462/468, 465/468 and 461/468 output lines while a routing proof and a Trojan class proof retried. A read-only native test of the affected panel passed Direct and Residential at 11:46:14 UTC. No manual reconciliation or weakening of publication evidence was used. The next five observations from 2026-10-18T11:47:21.199664+00:00 through 2026-10-18T11:48:21.670426+00:00 had 3/5 complete samples and all readiness/Output requests succeeded. Final output was 462/468, with 3 pending and 0 blocked; each profile had 78 eligible servers. Output latency was 37–53 ms (median 45 ms). Transient route/class proof failures can recur; this does not establish a permanent native configuration failure or a fault-free future.

Fresh deletion checks still show all 14 old profiles completed, 1101 confirmed removals and zero native references. Three active user profiles remain and both continuation canaries are absent. External OpenAI API calls in this recheck: zero.

A subsequent snapshot at 11:49:41 UTC (15:19:41 Tehran) recovered automatically to 468/468 outputs, all three profiles ready on 78 eligible servers, zero pending and zero blocked. All 234 profile runtime proofs and all 80 sampled routing proofs were fresh. This later complete snapshot does not erase the preceding transient gaps; continuous full publication coverage was not established.
