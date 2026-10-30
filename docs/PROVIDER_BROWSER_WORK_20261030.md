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

- Video and current-source review: complete.
- Country priority implementation: complete, exact-head regression gate required.
- Provider browser implementation: complete, final native/browser verification in progress.
- Isolated regression/network/browser tests: initial full pass; expanded locale/WebSocket checks in progress.
- Source/public CI, rollout and live acceptance: pending.

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
