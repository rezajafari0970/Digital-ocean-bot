# Ad asset speed tuning — 2026-10-11 Tehran

Runtime/source baseline: `1a0509dce27c2302d15cf9d789680fa27a5135d2`. This change is a supported persisted performance-profile operation, not a binary change.

The supplied 37-second video shows Qanari 4.2.1 (Lib v39, Xray v26.7.28), approximately 20 seconds in Requesting Ad before the first creative appears near 24 seconds. Video alone does not separate mediation, fetching, SDK callbacks and rendering. The Android application source is not present in this repository.

## Observed issue and operation

The fleet profile had fast_count=13, fast_share=80. On the sampled panel the fast and exploration balancers both had expected=12 for the same 12 candidates, so the advertised fast preference selected the same full candidate group. Native observer RTT was approximately 1512–2649 ms.

A single panel trial used the existing supported residential-performance-tune command with a ten-minute automatic restoration deadline: fast_count=3, fast_share=90. All candidates remain available; each connection has a 90% fast-lane preference, with 10% exploration among eligible healthy candidates. This percentage is a selection probability, not a bandwidth guarantee.

Trial panel: `b6a6388c-cc39-4d90-9a47-7b20a11f31ef`.
Experiment: `38f9c98e-0acb-4f95-8639-8e65087edce0`.
Trial/request: `0856bf85-c0a8-4962-b093-608dbdf97031`.
Supported fleet publication accepted at 09:10:34 Tehran, experiment version 6. Existing ownership, verification and recovery gates remain enforced.

## Bounded comparison

One panel, three sequential samples per static asset before and after; all 12 requests HTTP 200, 4,280,278 payload bytes total. No ad impression or click request was generated. No repeated full fleet native tests or unit/race suite was run for this profile-only operation.

| Static asset | Before median | After median | Observed reduction |
|---|---:|---:|---:|
| adsbygoogle.js | 4.462199 s | 3.365456 s | 24.58% |
| ima3.js | 4.968970 s | 4.572366 s | 7.98% |

This is a small sequential diagnostic, not a randomized controlled experiment, mobile-network benchmark or measurement of ad-ready/show-rate improvement. TLS-ready timing includes the preceding tunnel/gateway path; SOCKS-local lookup timing does not isolate remote DNS. CDN asset bytes varied slightly for adsbygoogle.js.

## Scope and remaining application evidence

Domain categories, explicit root scope, probe exceptions, DNS, protocol support and fail-closed behavior were unchanged. geosite:google and exact six-URL probe exceptions remain NOT implemented. The pre-existing UpCloud relay health limitation is not resolved by this selection tuning.

To attribute the remaining application delay, capture monotonic timestamps for VPN-ready, SDK initialization completion, load request, onAdLoaded/onAdFailedToLoad, show and impression; include sanitized ResponseInfo adapter latency. Check existing preload/single-flight handling and the actual SDK version before suggesting flags (initialization/loading flags already default true in GMA SDK 24+). Do not report static asset gains as faster ad rendering.

Official references:
- https://developers.google.com/admob/android/response-info
- https://developers.google.com/admob/android/optimize-initialization
- https://developers.google.com/admob/android/app-open

## Evidence and recovery

Private operational receipts and before/after measurements: `/root/backups/dob-ad-speed-20261011/`. `status.json` and `fleet-history.jsonl` distinguish current active-panel application from the stricter PUBLISHING/PUBLISHED lifecycle, which also retains obligations for surviving retiring/expired panels. Never erase those obligations or relax freshness to claim completion.

Use supported tune_restore with the exact current experiment version and tuning ID if restoration is required; inspect fresh state first. Original full profile is preserved in profile-before.json and tuning.before. No separate OpenAI API calls were made.

## Rollout receipt at 2026-10-11T09:14:30.174772+03:30

43/43 active panels have the candidate configuration with matching applied profile generation and routing performance generation. At this snapshot 42 were APPLIED and one was in routine APPLYING without error; a generation receipt is not a promise of uninterrupted current health. API, worker and panel-worker are active.

The lifecycle marker remains PUBLISHING: surviving RETIRING/EXPIRING/DELETING records and the strict concurrent freshness gate retain recovery obligations. No completed-lifecycle or app-ready improvement claim is made.
