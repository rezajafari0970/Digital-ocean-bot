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

### 06:04 UTC candidate and CI checkpoint

- Gate 5 PASS: all 323 required roots, zero skips; the isolation test repeated
  20 times. Full go test ./... and go vet ./... PASS (run 3). Gate 4 never started
  because the shell PATH omitted runuser; the explicit executable path fixed it.
- First source candidate 9c0d08a65d0d64abbdb78eb4b83b2c05c188b19d is draft PR32.
  Its tree exactly matches local checkpoint a8070b72. No source merge or release.
- Full export scan PASS: 1,838 files, 7,125,683 bytes; both live secret comparisons
  (15 and 12,121 values) and Gitleaks passed without logging secret values.
- Additional protection: a client must now explicitly send its settings revision
  for account/proxy updates. A cached older form cannot substitute the latest
  server revision and silently overwrite a newer draft. Updated fixtures obtain
  real revisions; unversioned-save rejection is exercised. Interrupted advisory-lock
  acquisition discards its SQL connection so an uncertain lock cannot reenter the pool.
- Final focused PostgreSQL/race/browser suites PASS at 05:59, including validated
  credential replacement and all country/legacy/settings cases.
- PR32 CI run40640451431 passed general code checks and three account-editor browser
  repetitions, then timed out in NewCloudProviderBrowser. Its stale fixture seeded
  only client cache, but the fixed editor now reads the authoritative account list.
  Fixture now supplies the account through HTTP and starts with a deliberately stale
  client cache. Browser commands have bounded diagnostic timeouts. A second assertion
  was updated to require unavailable saved choices remain in FormData AND prevent
  a changed-region invalid save from dispatching. Full provider browser PASS (14.940s);
  blocked-popup variant and updated-head CI remain pending. No assertion was skipped.
- Local ephemeral execution environment disconnected at approximately05:57. Work
  continues from the committed server checkout /root/projects/dob-country-final-20261028;
  code/review/test evidence remains on servers and GitHub. Live services unaffected.
- Final API review implementation2 still pending at06:01. Six later safety/test
  files and browser-fixture changes are subsequent deltas, not silently included
  in that already-submitted prompt. No original PostgreSQL cluster was restarted:
  PostgreSQL18 main is down; umbrella service is active/exited with MainPID0.

### 06:44 UTC second-review corrections (not deployed)

- PR32 head eba29fc3 CI run40641899256 PASS. New-cloud real browser PASS in
  both normal (14.940s) and popup-blocked (14.846s) modes. These precede this delta.
- API implementation2 completed, receipt resp_08f890e95102dd8a006ae18ab299e887d1866b9482511640a6:
  519,215 input (154,087 cached), 112,928 output (105,352 reasoning), 632,143 total.
  Recommendation BLOCK: eight high and four medium findings. They were accepted
  as release blockers; the draft PR and live revisions remained unchanged.
- H1: repository credentials, rendered session/port and route fingerprint now come
  from one SQL snapshot; admission compares that fingerprint before dispatch.
  Stale-session and same-proxy-ID endpoint regressions pass with zero sends.
- H2: production guards use native WroteRequest completion. Source-body Close and
  cancellation do not prove upload settlement. EOF marks only response completion,
  avoiding native HTTP/2 Close's cancellation/wait of an unfinished upload. Early
  errors lacking WroteRequest close the exact native connection before releasing.
  Real HTTP/2 EOF, explicit Close and cancel regressions all pass, including blocked
  exclusive route writes. First run exposed the EOF/Close wait; fixed, not skipped.
- H3: keeper uses a 60-second durable owner lease and 50-second work deadline;
  publications validate owner under the route transaction. No pooled connection
  is held during probes. Account listing drains rows before nested enrichment.
  Both public keeper/list endpoints pass with MaxOpenConns=1; stale owner rejected.
- H4: proof/observation expiry rechecked after allocation/constraint waits directly
  before commit. Test forces higher-country proof to expire behind the global port
  lock; the complete incumbent route/epoch/applied state is preserved.
- H5: mixed transport/observer errors and malformed IP evidence stay Unknown;
  explicit proxy authentication rejection remains provider-negative evidence.
- H6: existing generic AUTO may retain its legacy changing IP under higher-country
  Unknown, with isolation checked. Explicit/fixed/sticky identity rules unchanged.
- H7: UUID aliases canonicalized before route/proxy locks and writer tokens.
  Uppercase policy editor and route writer wait on the canonical in-flight lock.
- H8: invalid network modes rejected before replacement credential validation;
  validator independently rejects unsupported modes before constructing transport.
- M1: proof budget accounts for finite provider attempts/country count (minimum
  five minutes, 30 seconds per configured attempt/tier for probe/resumption margin).
  Eight providers x six 6.5-second failures reach AUTO rather than restart forever.
  Unknown still cannot authorize descent; these are finite driver budgets, not
  enumeration of an unbounded residential address pool. Longer lists imply a longer
  evidence window; retry scheduling remains separate and restarts after 30 seconds.
- M2: keeper health saves compare captured row version under FOR UPDATE. Three
  newer passive failures survive a late Unknown keeper result unchanged.
- M3: vultr-browser-session uses the exact captured secret version.
- M4: account-proxy test publication compares proxy settings and secret versions
  for both failure and success, checks affected rows, and returns conflict on change.
- Focused race run2 PASS: app 14.153s, admin/browser suites PASS, native HTTP/2
  three modes PASS, selector PASS. M4 helper/CAS tests and mandatory gate additions
  came after that run; full gate6 and final unit/vet remain pending.
- No original quarantined PostgreSQL runtime or database has been started.
  No merge, public promotion, live installation, or new cloud CREATE was performed.

- Implementation3 submitted once at06:44:37 UTC, gpt-6-astra max/pro, receipt
  resp_07fc94902f6a77f8006ae199d69e5887d197d273b361fac329; 25 current files,
  previous review and dispositions; 230,558 bytes, SHA256
  ad97217b3b65ad8d2054888b8529671434093905a68f9348f9f96dd8b7286371.
  No output-token/request/spending cap was added. Pending, no approval assumed.


### Video follow-up continuation (2026-10-28)

See `docs/VIDEO_FOLLOWUP_20261028.md` for the new 337-second recording,
read-only live diagnosis and phase-by-phase status. The candidate adds compact
native mobile plan labels with complete per-selection price/specification
summaries, visible legacy preference deduplication, and truthful profile-scoped
scheduler concurrency status. Final 337-root mandatory gate, full Go tests,
vet, browser cases and backup restore pass. New strongest-model OpenAI review
`video-followup1` failed with `credit_balance_exhausted` and no output. Source
remains a draft; neither runtime has been updated. Three ambiguous Kamatera
creates still lack terminal provider evidence; do not replay or clear them.
