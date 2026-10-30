# Provider browser and onboarding priorities — 2026-10-30

Base source: ec1c4521302a1fb6de7136e81d39cf515aa6e383. Base public runtime:
67f91d2886e0b2b6dec6754700066a35eb2fccae. This is new work, not a replay of the
completed October 29 rollout.

## Evidence and requirements

Reviewed the complete 69.985-second recording using all 70 one-second frames.
Video SHA256: a5cbaadadc0489bd4408e96e60bdfa465f1605d847af2341922351cd56c6d0fd.
The recording is effectively silent. 00–35s shows dashboard/account diagnostics;
36–51s demonstrates a single country selector during creation; 52–59s shows
account cards without provider-browser actions; 60–66s shows delayed Build
Activity loading; 68–70s shows output controls without opening an output.
No new output failure is demonstrated. Three prior Kamatera creates remain
unknown; the video does not establish a terminal provider result.

1. Three country priority slots, initially AUTO, persisted atomically with the
   account and applied before preview/authorization. Explicit slots are tried in
   order across the assigned eligible proxy pool. AUTO placeholders never add
   an unrequested arbitrary-country fallback. Unknown evidence blocks descent.
2. Provider-registered website entrypoints and a real remote browser. Each open
   receives a disposable profile, isolated cookies/storage and an authenticated
   owner-bound control channel. No target HTML executes in the panel browser.
3. Browser networking must be constrained by the OS, with only a guarded proxy
   bridge available; missing sandbox/proxy/country verification blocks launch.
   Route drift closes existing tunnels and the browser session. No direct retry.
4. Preserve existing account rules, credentials, output identities and uncertain
   paid-operation reservations. Do not start the quarantined original cluster.

## Stage status

| Stage | Status |
| --- | --- |
| Video/source review | Complete; 70 one-second frames reviewed |
| Three AUTO-default country priorities | Implemented; preview, authorization and save share policy |
| Isolated provider browser | Implemented for all seven registered providers |
| Native browser/mobile controls | Passed in independent CI |
| Final source regression gate | Passed: 41032403736, source 93ab72f803386287de3458bff984a09335c93721 |
| Public test and Ubuntu install/reinstall | Passed: 41032489569, candidate 68893142fff30c5567afdc7c3233ee9da0004721 |
| Exact export live-secret scans and gitleaks | Passed against both installations, no secret values logged |
| Atomic main/stable publication | Passed: 41033585970; public revision 68893142fff30c5567afdc7c3233ee9da0004721 |
| Runtime rollout | Blocked before any change: insufficient disk reserve |
| Live provider-site acceptance | Pending runtime installation |

The original development device is offline. Heavy tests must use independent CI,
not the 4 GB production host. The retired Vultr console is not a safe networking
or session-isolation implementation and must stay retired.

## Development checkpoint

Draft PR34: `feature/provider-browser-priorities-20261030`.
Second candidate `8baa51753db8e900a2c56c3435ca4e911a46920c` passed source CI
41027518593: complete Go tests/vet, existing real-browser UI tests, two actual
systemd-isolated Chromium sessions with separate cookies/storage, 351 mandatory
regression roots with zero skips, 20 recovery/lifecycle repetitions and a verified
restore of schema 206. The first candidate had a CDP return-arity compile error;
it was corrected before the successful second run.

Subsequent work adds mobile touch input, a standalone browser-control UI test,
country-derived browser locale/timezone, native WebSocket/input checks and
additional authorization lifecycle fences. These changes require a new exact-head
CI result before publication. No public branch promotion or runtime update has
occurred. Build Activity latency and the three prior unknown Kamatera receipts
remain separate observations, not closed by these feature tests.

Design references: provider-registered HTTPS entrypoints; systemd.exec documents
PrivateNetwork, DynamicUser, PrivateTmp and the restrictions used by the worker:
https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html
Chromium control APIs are pinned by go.mod and used through chromedp:
https://chromedevtools.github.io/devtools-protocol/

Third candidate e9aaba86 passed mobile UI and source checks but failed the expanded
native locale/WebSocket assertion. Fourth candidate explicitly binds the headless
browser Accept-Language/navigator language, records actual fixture state on failure,
and verifies the installed socket-activated sandbox during install/reinstall.
Rollback now preserves browser socket enablement and closes old browser workers.

Fourth candidate 3260fe93 passed the expanded native sandbox: HTTPS, two WebSocket
tunnels, input, locale/timezone, separate cookie/storage identities, two rejected
destinations and zero direct host requests. All 351 roots and schema-206 restore
passed again. The next public install identified missing Chromium libraries on
minimal Ubuntu and an outdated permissions-test fixture; both are corrected and
the root permissions checks are now also required by source CI.

Live rollout preflight currently blocks on disk reserve: database 3,328,063,167
bytes, required free 10,521,060,413 bytes, observed free about 7.67 GB. No runtime
swap or schema migration has started. Do not lower the native retention guard or
delete unrelated application data, backup retention or incident evidence.

## Final candidate and continuation

Source PR: https://github.com/rezajafari0970/Digital-ocean-bot/pull/34
Public candidate: 68893142fff30c5567afdc7c3233ee9da0004721.
Install and repeat-install both passed on an independent Ubuntu 24.04 guest.
The installed socket-activated DynamicUser runner launched actual Chromium,
returned an isolated browser image with every attempted egress tunnel denied,
and the retained administrator/environment/key fingerprints remained unchanged.
The installation smoke uses the real wire error contract; internal worker errors
are deliberately redacted by that protocol.

At 2026-10-30 09:19 UTC both live instances still served 67f91d2886e0b2b6dec6754700066a35eb2fccae.
All six runtime services were active with matching executable hashes and zero
restarts; all ten readiness checks passed. No browser runtime package, service
unit, schema migration or runtime swap was applied to the live host.

Before deployment, add storage or reclaim explicitly reviewed reproducible data;
keep the three-times-database-plus-512-MiB production reserve, backup retention
and original-host quarantine. About 6 GB additional free capacity provides room
for the present deficit plus browser/build/backup work, but remeasure immediately
before rollout because the database and native reserve can grow.

Resume with fresh independent instance baselines, then bounded default-instance
update (which installs the shared browser runtime/socket), followed by bounded
production update. Verify schema 206, executable/static hashes, saved settings,
output identities, all readiness checks and actual provider-browser routing.
An existing session never changes country silently; failed route proof closes it.
No billable provider create should be replayed to test this feature.

The browser supplies navigation, tabs, text input and mobile scrolling. Uploads
and downloads are unavailable. Sessions are disposable and last at most 15 minutes;
the two-minute idle limit applies to absence of control-channel activity. Native
fixtures verify isolation, not every third-party site's login or anti-bot behavior.

Final source CI 41032403736 passed all 351 mandatory roots with zero skips, the
expanded native browser gate, mobile controls and schema-206 restore. PR34 merged
as fc49e99ebe8a37553e6b8b948b9da3b3e4ef4d82.

Atomic promotion run 41033585970 passed. Both public main/stable refs were re-read
and matched candidate 68893142fff30c5567afdc7c3233ee9da0004721; its exported file
manifest matches the tested immutable source export. Live rollout remains blocked.
