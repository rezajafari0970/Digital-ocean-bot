# Video follow-up — 2026-10-28

## Current checkpoint

Source parent: `d6a2a487233abf1635b4242fec420c3ff00ca76e` (draft PR32).
This follow-up continues the country-priority and account-save work; it does not
claim that the candidate is installed. Both live instances still report public
release `8515d676c5f64cbd7d51eacfa8f15a99add67053`.

| Phase | Status | Evidence / remaining work |
|---|---|---|
| Recording inspection | Complete | 337.432-second Android recording; whole-timeline sampling, dense save/reopen review, and final frames inspected. Silent audio. |
| Live diagnosis | Complete for observed blockers | Read-only production queries establish five overdue Kamatera servers, three unresolved creates, and no fresh output snapshots for that account. |
| Mobile account changes | Implemented | Compact native plan options; complete per-selection specs and region price below each field; disabled duplicate alternatives; legacy repeated preferences normalized visibly before saving. |
| Scheduler diagnosis | Implemented | Profile-scoped concurrency now qualifies scheduler readiness separately from provider permission. Saturated unknown creates explain excluded expired servers. |
| Deterministic verification | Passed | Final product candidate: 337 required regression roots, zero skipped; browser fixtures, all Go tests, go vet, and disposable backup restore passed. |
| Independent OpenAI review | In progress | After the user confirmed funding, one fresh `gpt-6-astra` max/pro request was accepted at 09:44:50 UTC. Follow its existing receipt; do not duplicate it. |
| Public candidate installation | Fixing test environment | Exact exported candidate `bb405b4d` passes the manifest and live-secret scans; hosted Ubuntu 24.04 kernel preflight stopped installation. Guard remains unchanged. |
| Merge / stable release / deployment | Pending | No stable promotion, application update, manual create/delete, or alteration of unknown-operation evidence performed. |

## Recording and root causes

- 00:50–00:56: Android's platform-owned plan picker wraps long specifications and
  price strings over other rows. CSS on the page cannot reliably repair that
  native popup. The option now carries the plan ID, RAM and CPU; a normal page
  paragraph carries the full specification and current primary-region price.
- 01:04–01:12 and reopen: repeated plans/images appeared saved, then optional
  slots disappeared because alternatives are a set, not probability weights.
  Prevent duplicates while choosing, normalize legacy repetitions on opening,
  explain the rule, and keep selected options enabled for FormData.
- 01:32–01:56: unknown provider creation timestamps and conflicting readiness
  indicators. Existing PR32 fixes the zero timestamp. This follow-up fixes the
  missing scheduler concurrency qualification.
- 02:00: Kamatera output is empty. Production evidence establishes expiry, not
  an output parser failure. Publishing expired credentials is not a repair.
- 02:57–03:08: DigitalOcean editor shows legacy repeated preferences and an empty
  primary region with fallback selections. Keep saved unavailable choices and
  authoritative revision checks from PR32; do not change the user's current
  settings while diagnosing the recording.
- 04:32–05:05: worker retry counts are not counts of billable provider requests;
  pre-SSH diagnostics need the provider-create error. PR32 already separates
  dispatch evidence and worker attempts and includes mobile diagnostics fixes.
- 05:29–05:37: the recording reports 48 ready servers and published config
  coverage for those servers. It does not establish that all providers or
  profiles are broken. Historical installer errors are distinct from live state.

## Why Kamatera remains blocked

At 07:52 UTC (11:22 Tehran), one managed server was EXPIRING and four were READY,
all beyond recorded expiry. Three deployments remained CREATING; their CREATE
operations retained one recorded create attempt each and unknown outcome. This
counter alone is not packet-level proof of dispatch. No durable
provider task ID or server UUID was bound to these three intents. Their names
and region intent remain in the private operational evidence.

`ProcessLifecycle` deliberately waits when pending deployments exist. The
scheduler's profile has all three concurrent slots occupied. The former
account display only considered provider permission and desired occupancy, so
it misleadingly said Ready to build. The new display reads the same active
states and profile scoping as scheduler admission, preserves provider permission
as a separate field, and reports a known zero immediately when all enabled
schedules are saturated. Another profile/account with room remains unaffected.

There is a second restriction: expiry retirement below Desired with usage-only
capacity was rolled out for Linode only. Expanding destructive retirement to
Kamatera requires ownership, outstanding-operation, proxy generation, backfill,
and failure-path review. This change does not silently widen that policy.
An empty queue or inventory is not conclusive proof that an ambiguous billable
request failed. Do not resend it, clear its reservation, or mark it successful.
Provider-confirmed evidence is needed to resolve these three intents safely.

## Safety boundaries and verification

Allowed changes are account form presentation, read-only account build status,
and their browser/PostgreSQL regression tests. No schema, billing, credential,
proxy mutation, provider-create, lifecycle deletion, or output expiry rule is
changed by this follow-up. Original quarantined production processes/database
remain untouched.

Focused coverage includes all seven provider editors; current primary and
alternative regional prices; unknown pricing; selected unavailable choices in
FormData; distinct saved alternatives; native picker duplicate prevention;
legacy repeated preferences; responsive Save controls; late save responses;
unknown-create concurrency, profile/account isolation, and preservation of
operation state and actual dispatch count. Chromium uses mobile viewport sizes;
this does not substitute for acceptance in the Android native picker itself.

Private logs: `/root/dob-video-followup-20261028` on the development host;
read-only live evidence under the same directory on the runtime host.
OpenAI receipt is stored by the existing durable review ledger. The request at
07:59:53 UTC (11:29:53 Tehran) failed before producing any review output. Do not
mark the independent-review phase passed or keep resubmitting without a credit
change.

Read-only native-provider follow-up at 08:08:13 UTC (11:38:13 Tehran):
HTTP 200, valid JSON, five inventory servers, zero matches for the three retained
intent names, and an empty task queue. This supplies no conclusive terminal
receipt; no mutation or reservation release followed.

## Deterministic results

- Final account editor/new-provider mobile browser fixtures: PASS, including
  saved unavailable values, legacy repetitions and complete alternative prices.
- Final transport/fleet PostgreSQL/race gate: 337 mandatory roots PASS; zero
  selected test skips; recovery/lifecycle isolation repeated 20 times.
- Full `go test -p 1 ./...` with browser opt-in: PASS.
- Full `go vet -p 1 ./...`: PASS.
- Linode/UpCloud native Chromium regression fixtures: PASS. Their old hardcoded
  Snap path, pre-revision cached-account fixture, and inline-price assertions
  were repaired so they actually exercise the current UI. No assertion was
  dropped to permit a failure.
- Backup restore drill: PASS through migration 204; encrypted identity and key
  restoration, unknown intents, sequences, amcheck, damaged-archive rejection,
  atomic failed restore and interrupted transaction rollback verified. The
  drill used a disposable socket-only database and no production database.
- Changed-source Gitleaks: PASS. Known-live-secret scans against both live
  instances: PASS (12 files, 320,654 bytes at the scan checkpoint; compared 15
  and 12,288 private values in memory without logging values).
- Two production installations independently checked at 08:08 UTC (11:38
  Tehran): all six services active, each readiness endpoint reports all five
  checks healthy. Both remain on release `8515d676`; this is availability
  evidence, not evidence that the new UI is deployed or Kamatera is repaired.

## Next action

The deterministic checks are complete; keep the evidence with the candidate.
The user restored OpenAI API credit. Poll the accepted final review of product
commit `f6bcaf848852bbe2048f84ec99cabd9d1f6e69d1`, resolve findings, then use the guarded source/public release process,
canary installation, runtime checks, and rollback checkpoints. Separately obtain
conclusive Kamatera task outcomes; never infer them from empty inventory.

## Funded continuation and installation environment

- Source CI [40655881902](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40655881902)
  completed successfully for `f6bcaf84`.
- Funded review receipt: `resp_06ab7bdd841edd9b006ae1c412f07087d18916f4000c1be13c`.
  At 10:04 UTC it remains in progress, with no completed review or reported usage.
- The full 1,841-file export passed both live-secret comparisons and Gitleaks.
  Public tree `8848d406edfcc1add5c81cd1d9d9db648d52cc53` exactly matches the scanned
  local export. No live secret values were logged.
- Public install run [40668782986](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40668782986)
  stopped at the unchanged host-memory preflight, before installation. An earlier
  country candidate had the same runner failure: Azure kernel `7.0.0-1017-azure`,
  `CmaTotal=0`, `CmaFree>0`, and no active KHO mitigation.
- Installation CI now requests a 22.04 host, verifies it with the same guard,
  and installs/reinstalls the exact candidate in an unprivileged Ubuntu 24.04
  LXD systemd guest. OS, architecture, init process, commit, source manifest,
  restrictive root permissions and the existing preservation smoke checks remain
  required. This changes the test environment, not the production guard.
