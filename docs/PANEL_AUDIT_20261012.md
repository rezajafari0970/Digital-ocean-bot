# Panel audit, 2026-10-12

Baseline source: 06f69a121f156203bafe42ce2087f09755012394. Runtime before release: 990a4120d52e132581f8c6070c67f1ad55d4cd13.

## Evidence and changes

The new 105.65-second mobile video was inspected through its final frames. Panel Settings finishes loading. Live provider accounts have no token-invalid or provider-unknown state; the previously blocked Sara accounts are absent and the deletion queue is empty. No real account was deleted as a test. UpCloud relay paths recovered between recording and inspection; this does not establish permanent connectivity.

Capacity previously filtered observations older than 30 seconds out of the entire census, showing only the rotating recently-polled subset. The API now enumerates eligible panel inbounds, reports missing/future observations as unknown, includes last observations with explicit age and coverage, and labels sums as last observed. Both snapshot writers use the same class-aware calculation: preserved manual users and surplus users of one class cannot conceal a deficit in another class. Missing required traffic data retains the prior observation rather than claiming new certainty.

Residential status now uses the completion gate's exact generation/revision and 60-second native-proof freshness predicate. Future proof timestamps are rejected. Assignment diagnostics identify missing, stale, unverified, or mismatched proof without forcing ACTIVE or RESTORED.

The initial candidate opted the native HTTP driver into four parallel read-only route checks. Production inspection disproved this capability: the panel closes a shared gRPC connection after routeTest and overlapping calls can fail inside an HTTP 200 response. That runtime was restored to exact prior990a412. The corrected Sanaei HTTP driver is serial. Eight serving workers instead share the due queue, preserving durable reservations, cursor fairness after selective SQL failure, due rechecks and per-panel locks. Fixed hash-bucket imbalance no longer strands independent panels. Retirement retains a separate consumer. Only independently proven drivers may opt into the generic bounded parallel helper. The full route matrix and serial status/DNS/relay checks remain required; configuration writes and restart behavior are unchanged. The executor cancels and joins pending reads before returning ownership. Shared CSRF/session state is protected, relogin storms are coalesced, and lock waits honor context cancellation. Synthetic 30 ms-per-request testing measured the identical 34-request matrix at about 1.07 seconds serial and 0.32 seconds concurrent. This is not a production latency claim and is not applicable to the deployed Sanaei serial contract.

Dashboard, routing and performance refreshes use owned, single-flight reads; stale responses cannot overwrite a different page or global performance state. Last good data survives transient failures. Existing mobile card-table styles show capacity, health and recovery diagnostics without excessive horizontal scrolling.

## Validation and release gates

`tools/panel-audit-acceptance-race.sh` requires an isolated PostgreSQL database, binds all changed source to its accepted test receipt, and runs the full Go suite, race checks, real Chromium refresh/deletion checks, and installed-core policy/traffic tests on both supported Xray versions. `tools/panel-audit-review.py` sends the exact accepted source for a measured API review and retains the response before parsing. Failed attempts remain in `/root/backups/dob-panel-audit-20261012`; only a matching PASS permits release.

Publication requires a clean commit, unchanged canonical/runtime baselines under the shared publication lock, matching binaries/static files, and post-release readiness and settings checks. No schema change is required. The previous runtime is rollback-compatible. User profile targets, quotas, intervals, execution controls, proxy selection, category/UDP policies, and protection settings remain user-owned.

## Limits

Historical backups and external provider billing records are not deleted by application account deletion. Current live proof ages and provider availability can change independently of code. Strict recovery remains pending until every surviving obligation has valid current proof or confirmed deletion. No repository test can certify all future provider/network behavior; release evidence must distinguish tested logic from observed production outcomes.

## Native contract correction

Release b0bb9c9 was rolled back after the post-publication native API check found the shared gRPC close behavior. No configuration template, proxy policy, proof freshness window, or recovery record was cleared. All 49 serving routing records subsequently returned to APPLIED with no errors on the restored runtime. `panel-native-contract-20261012` binds the correction to new affected-package race and HTTP-contract tests and a fresh API review. Prior comprehensive tests are reused only for verified unchanged bytes; no claim is made that the rejected concurrency strategy passed production acceptance.

## Admission headroom

Post-release evidence found native proof age still reaching 80 seconds because bulk capacity and policy scans saturated the shared 10-slot budget (25 waiters), while the 40-connection SQL pool had zero connection wait. A two-slot polling class now gates global Reality, capacity fill and capacity cleanup before the unchanged shared admission budget. Routing, client mutation and retirement retain their original bounds and guards. The polling class is visible in heartbeat metadata; blocked polling acquires no shared slot or SQL connection. A contention/cancellation regression requires all eight proof consumers to enter despite twenty queued polls, with no slot leaks. Production proof latency remains an observed release gate.

## Verified observation reuse

The admission correction reached genuine RESTORED with 50/50 surviving obligations verified and zero routing errors. The two-slot polling class exposed a second issue: route checks had current full native inventory while independent inventory and capacity polling lagged. Routing now publishes its actual post-proof snapshot through the existing normalized inventory store and bulk capacity observer, after native proof, membership and policy fences. This adds no native requests or mutations. Missing required traffic retains both previous capacity counts and their observation timestamp; failed native proof cannot fabricate fresh inventory. All writes complete before the route receipt transaction. Unmanaged legacy inbounds keep their existing observer. Publication remains conditional on fresh affected-package race/PostgreSQL tests, exact-source API review and live freshness checks.

The first API review rejected the candidate because native stream settings can be a JSON object or an encoded JSON string. The observer now accepts both existing native formats. The PostgreSQL regression removes prior evidence before testing the encoded form, then verifies missing traffic preserves its timestamp. Rejected receipts are retained; changed source requires new acceptance and review.

## Completed tuning and later enrollment

Release 1be2fab reached 51/51 active servers, all 51 native routing proofs fresh, zero inactive/degraded/broken servers and zero route errors. Last observed eligible capacity was 245/245 with zero deficit. Deployment cancellation initially interrupted eight proofs; durable reconciliation resumed them without manually clearing state. A remaining display error counted two later fleet assignments against the old completed tuning identity, producing 49/51 pending details beside 51 verified current assignments. The diagnostic now returns no active progress for terminal RESTORED/PUBLISHED cycles, while keeping their history and all current fleet proof counts/blockers. Active recovery predicates and state transitions are untouched. PostgreSQL regressions cover both terminal phases, enrollment after completion and genuine new-server stale proof remaining visible.

The same live pass found two newly provisioned, eligible panels without initial inbounds. Global Reality alone started its 20-second deadline before waiting for shared polling admission; queued work could expire without executing or producing a native diagnostic. It now uses the existing bounded batch helper and starts that unchanged deadline after admission, as capacity handlers already do. Class limits, global budget, per-panel locks and native guards remain intact. Actual initial configuration creation remains a post-publication observation gate.

## Final verified publication

Code release `9e59e3a45f90e6b98043c2bae66b88b8cad352f8` was clean-built and published under the shared deployment lock. The accepted source digest is `24debb48e2e37fa5154842ee248ff2d59ccd6802f4805d9fe81723db3cd8ebbc`. Fresh affected-package race/PostgreSQL gates and actual exact-source API review passed; inherited broad gates apply only to checked unchanged source bytes. All three services match their manifest hashes, readiness is 5/5, nine static files match, and restart counters are zero. Configuration/environment/systemd hashes matched before and after publication.

Final observation at 2026-10-12T04:15:08.475343+03:30 (Tehran): 48 active, 0 inactive, 0 provisioning, 0 broken, 0 degraded, 2 pending deletion, 50 total. All 48 serving routes have current native proof; route errors are zero. Performance remains RESTORED with no invented active progress for completed tuning. Account deletion queue is empty. 8 accounts are enabled, 6 have genuine capacity-full evidence, 0 are broken and 0 unknown.

Eligible capacity rows: 45; observed: 45; within the explicit 30-second window: 34. Last-observed totals: target 225, active 217, deficit 8. No stale observation is presented as a new native read.

Both previously empty Vultr panels completed three-round native TLS target observation and now have a selected target plus an enabled native VLESS/Reality inbound on port443 with clients. One automatic bootstrap attempt logged an SSH channel-open failure; the existing controller resumed from persisted selection and created the actual inbound without manual SSH, forced status flags or relaxed TLS/proof guards. New client filling retains configured intervals and class targets. Four post-release windows and subsequent bootstrap convergence observations are retained in the evidence directory.

External OpenAI API usage for this panel audit: 11 generation requests, input 185260, output 11866, total 197126 tokens. This includes rejected candidates and reviews; no monetary cost is asserted because receipts contain no charge. Separate earlier account-cleanup usage was 4 requests / 175760 tokens; combined documented totals are 15 requests / 372886 tokens. Raw receipts and release evidence are retained at `/root/backups/dob-panel-tuning-display-20261012`.

This final documentation-only checkpoint does not change the deployed executable source. Historical backups and provider billing records are not erased by operational account purge. Previously invalid-credential accounts cannot be claimed cloud-deleted from local removal alone. No live account was destructively deleted as a test.
