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
