# Account settings, video findings and country selection — 2026-10-28

Status: implementation in progress; no new product release deployed yet.

Baseline: source `9430d78a479b544db465f961a5aaf14bcbb0be4a`, installed public
release `8515d676c5f64cbd7d51eacfa8f15a99add67053` on both secondary instances.
Original server is online; fresh isolated worktree created there. Its corrupt
database and duplicate workers remain quarantined; gateway/backup role preserved.

## Evidence and stages

| Stage | State | Evidence / remaining work |
|---|---|---|
| Recover baseline | Complete | Verified source worktrees and original server services |
| Video examination | In progress | 341.937-second silent recording; entire timeline sampled every 5 seconds, account edit at 0.5-second intervals |
| Independent API review | Complete | GPT-6-astra max/pro response completed 04:03 UTC; 302,206 input / 100,073 output tokens (77,502 reasoning); review 58,299 bytes; legacy ledger untouched |
| Settings durability | Focused tests pass | Revision conflicts, omitted fields/interval/batch/cadence preservation, atomic credential/update and ordinary creation, active failover retention, unavailable IDs, long pools, lifetime precision, modal ownership |
| Country priorities | Core implemented; integration pending | Pure country-first resumable selector passes race tests; unknown observations cannot authorize lower tiers; driver registry and world catalogue added; DB policy/runtime/API/UI and native acceptance remain |
| Regression / release | Pending | Meaningful browser, DB, concurrency tests; full source and public release gates |
| Runtime acceptance | Pending | Both runtime instances and original gateway; settings/credential preservation evidence |

## Video observations, without conflating history and current state

| Time | Observation | Interpretation / action |
|---|---|---|
| 00:00–00:15 | Provider observations and dashboard | Distinguish transport health, provider capacity and scheduler readiness |
| 00:17–00:30 | Card regions differ from edit form; same proxy in three slots | Card uses actual inventory regions, form uses desired regions. Clarify labels; prevent duplicate choices masquerading as independent failovers |
| 00:30–00:49 | Plan #2 selected; identical OS in all three slots | Arrays deduplicated on save; make uniqueness explicit, prove save/reopen preservation |
| 00:50–01:05 | Kamatera creation date 01/01/1 | Unknown provider timestamp rendered as Go zero time; fix at API/UI boundary |
| 01:05–01:15 | Unknown creates/reconciling | Preserve durable single-send operation journal; never assume a timed-out CREATE failed |
| 01:20–01:45 | DigitalOcean capacity full | External capacity restriction; must not bypass admission guard |
| 01:50–03:10 | Linode/OVH history and pending build | Historical errors are not current failures; clarify diagnostics |
| 03:25 | Base proxy country differs from account identity | Base endpoint and account sticky session are separate observations; clarify labels |
| 03:35 | provision_run_not_found | Known deployment has no provisioning run yet; return its deployment-stage diagnostics |
| 03:45–04:20 | Wide diagnostic tables unreadable on mobile | Responsive labelled rows, preserve full diagnostic evidence |
| 04:25–04:40 | High attempt count for unknown CREATE | Worker reconciliation attempts differ from actual provider dispatches; label accurately |
| 04:45–05:15 | Residential/config current and retired records | Inspect policy revision and retained historical metadata; do not delete evidence |
| 05:25–05:35 | Update status transitions then dashboard | Verify update receipt and current release; no assumption of lost settings |

## Existing external blockers

No saved AWS credentials exist for the failed wizard. The observed STS
InvalidClientTokenId/403 cannot be resolved or retested without valid credentials.
Three Kamatera CREATE operations remain unknown, each with exactly one actual
dispatch. Neither generic server errors nor an empty inventory prove non-creation.
These blockers remain open until authoritative provider evidence is available.

## Checkpoint 04:16 UTC

- Focused real PostgreSQL race tests and both account browser suites PASS on the
  original server using an isolated socket-only cluster (13.104 seconds). These
  tests never connected to the quarantined database or live secondary database.
- Browser coverage includes all seven cloud providers, retained unavailable
  catalog choices, four assigned proxies, non-dropdown saved concurrency,
  second-level lifetime precision, late dialog opening and late Save response.
- Settings test setup initially exposed two invalid fixtures (inconsistent scalar
  image alias, missing profile foreign key); corrected before acceptance.
  Snap Chromium could not start; the release-pinned verified Chrome binary passed.
  A deferred-response test initially awaited its own gate; corrected and rerun.
- Pure selector race tests PASS. Core is not connected to live account routing yet.
- API review findings are tracked against actual code: transport epoch triggers
  already cover profile/session/IP changes; database unique indexes already
  protect IPv4 and /24. Do not misreport those as absent safeguards.
- Additional source changes (not deployed): canonical collision configuration,
  collision admission fail-closed, observer-error preservation, two reconnect
  checks before legacy failback, complete sticky ASN publication.
- Ordinary account saves keep the existing 30-minute lifecycle minimum explicit;
  invalid values are rejected rather than silently clamped. No policy weakened.
- Still required: complete revision-fenced country-policy integration, exact
  AUTO compatibility, provisional candidate stability, all writer/admission/cache
  paths, policy UI/status, old-worker/rollback protection, independent final API
  review, full source/public gates, promotion and runtime preservation checks.

### 05:03 UTC implementation checkpoint (not deployed)

- Policy API routes and independent per-account ordered-country UI are wired;
  registry-backed proxy driver selection includes generic, fixed and suffix-session.
- Ordered keeper now uses immutable configuration/secret version snapshots, finite
  per-driver country sweeps, persistent checkpoints, separate candidate probes,
  reconnect checks, 10-second spaced failback, transaction fences and atomic
  identity/route/generation publication. No live account policy has been changed.
- Default AUTO keeps the legacy selection path. Its observation writes now compare
  the captured configuration/route/epoch under short transaction locks; a late
  observation cannot publish after a credential or active-proxy edit.
- Real Chrome country-editor conflict test exposed a shared Save-button handler
  from the prior account editor; fixed and verified. Fixture-only test defects
  (duplicate unique proxy names and missing SQL query parameters) were corrected.
- At 05:02 UTC focused race tests PASS: adminapi 14.468s; app 3.585s;
  proxyselection 1.008s. Includes real CONNECT/TLS candidate checks, independent
  reconnects, wrong-country/rotating/auth/observer cases, two-account /24 claims,
  stale candidate/configuration rejection, failback/no-churn, old AUTO writer fence,
  settings preservation and real mobile browser suites. This is NOT full release CI.
- Additional changes after that accepted run await verification: atomic ordinary
  proxy creation; proxy-test revision CAS; desired-pool consistency in direct network
  assignment; deep-copying the pre-edit slice snapshot so equal-length region/plan
  changes correctly reschedule; rejecting invalid/truncated choices explicitly.
- Independent API review `implementation1` submitted once at 04:54:39 UTC using
  gpt-6-astra max/pro, background mode, no old request/spending cap. Receipt
  `resp_04141f50ec49bc5b006ae1800fbb1487d1b2d2326063d7e5f4`; still in progress
  at 05:02:59. It reviews a checkpoint preceding the newer legacy-fence changes.
- Remaining: adversarial review dispositions, strict AUTO compatibility/fixed-driver
  and in-flight credential tests, complete provider-readiness audit, full native gate,
  video residential/update-state verification, original-source update, source/public
  publication and guarded installation on both healthy runtime instances.

### 05:40 UTC review and test checkpoint (not deployed)

- Independent API review implementation1 completed: 195,865 input tokens,
  109,610 output tokens (104,472 reasoning), 305,475 total. Its 15 findings
  are tracked below; no inferred API dollar cost is reported.
- Added version-matched proxy secret reads before gateway creation across
  runtime, country probes, previews, replacement-token validation and proxy tests.
  Proxy edits use a secret-version conflict check and never persist an omitted
  password from an earlier read. Native credential-race regression passes.
- Country negative-evidence lifetime is now separate from retry scheduling. A
  deterministic 7-second/session, 22-second/tick regression reaches AUTO after
  exhausting both countries, then retries the preferred country after cooldown.
- Saved candidate sessions are consumed once per invocation. Missing/invalid
  country observations and malformed/non-success geo responses are inconclusive.
  Publication checks observation age and higher-country evidence under the lock.
- AUTO continues through legacy maintenance, including its learned-country
  failback; explicit-country reconnect requirements do not alter generic AUTO.
- Driver credentials are rebuilt from the raw base username; country targeting
  and dedicated port changes are capability-gated. Independent non-idempotent
  driver regression is registered for the full gate.
- Request admission now validates through its already-held SQL connection. The
  route is held through request upload and response-body completion, including
  early response headers; passive observations run after release. Single-connection
  PostgreSQL/streaming regression passes.
- Shared-proxy invalidation uses stable account order. Acquire now loads full
  persisted circuit/health counters. The concurrent /24 test uses distinct sessions
  and checks the exact IPv4-/24 uniqueness constraint.
- Focused run 3: admin/browser suites passed (16.670s); one new saved-candidate
  test had an incomplete checkpoint fixture (missing scope), which was corrected.
  Do not count this whole run as accepted. Full gate 3 runs on a fresh candidate,
  with 323 mandatory roots and no skip allowance. Its outcome is still pending.
- Full gate 1 failed only the three account-purge fixtures trying to write the live
  browser-session directory. Added a process-scoped session root used consistently
  by CLI/purge; the gate supplies a private root. No live permissions were weakened.
  Full gate 2 was stopped by a missing REQUIRED map key, fixed before gate 3.
- Final independent API review implementation2 submitted once at 05:40:01 UTC:
  model gpt-6-astra, reasoning max/pro, no previous request/spending cap; receipt
  resp_08f890e95102dd8a006ae18ab299e887d1866b9482511640a6. Input includes 66
  current source files plus the earlier review, 596,126 bytes, SHA256
  45deb9d16bc118fa5e5ef98056433b27be462846051e043a204fd0649caa6e27. Pending.
- Read-only video follow-up: last residential rollback occurred October 18;
  current routing remains enabled, fleet enabled, 53 live panels APPLIED. UI now
  dates and labels publication history. Update job succeeded October 28 03:10:22–25
  from public8515d676 to the same version; no stuck live update was established.
- Release, source/public merge, installation on both runtimes and preservation
  checks are still pending. Original quarantined database remains untouched.

### 05:49 UTC validation checkpoint

- Gate 3: 322 required roots PASS, zero failures, one skipped new proxycontrol
  test because its legacy DOB_E2E_DSN fixture was not configured. Gate correctly
  rejected this run. Moved the health-history test to the existing isolated
  per-schema app fixture; no production behavior changed.
- Gate 4 / unit 2 / vet 2 did not start (runner PATH omitted runuser; exit127).
  Gate 5 and unit/vet 3 use its explicit /usr/sbin/runuser path. Still pending.
- Current local gate-harness 10, updater 11 and bootstrap 25 tests PASS.
- Changed-source scan PASS against both live installations: 63 files, 613,468
  bytes, 15 and 12,101 private values compared in memory; no values logged.
  Gitleaks PASS. Full exported-candidate scan remains a release requirement.
- Final API review implementation2 remains in progress. No merge or deployment.
