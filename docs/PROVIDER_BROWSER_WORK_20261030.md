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
- Country priority implementation: in progress.
- Provider browser implementation: in progress.
- Isolated regression/network/browser tests: pending.
- Source/public CI, rollout and live acceptance: pending.

The original development device is offline. Heavy tests must use independent CI,
not the 4 GB production host. The retired Vultr console is not a safe networking
or session-isolation implementation and must stay retired.
