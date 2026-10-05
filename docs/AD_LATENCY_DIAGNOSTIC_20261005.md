# Residential ad latency diagnosis — 2026-10-05

Status: current upstream latency and intermittent transfer failure reproduced; app-specific ad failure not yet isolated. This is a diagnostic checkpoint, not a performance-fix acceptance.

## Baseline and scope
- Canonical HEAD before diagnostics: 00a2b5ab63976544d003a9b0b7230afa9f821d7f; clean and equal to origin/checkpoint/final-e2e-20260929.
- API and Worker active. No product source, routing policy, provider credentials, generation policy, execution gate, or production binaries changed.
- Measurements: 09:47–09:53 UTC.
- A current Residential Output credential was used privately for panel 024a75e3-893d-4642-81a3-30d4f8641302, the first server shown in the later V2rayNG portion of the video. This does not prove Asho's earlier temporary tunnel used that panel.
- Temporary localhost Xray client, credential file, admin session and share were cleaned by the diagnostic harness.
- Existing Ads + temporary BrowserLeaks TCP routing and Residential UDP block retained.

## Video evidence and its limits
The 81-second screen recording was sampled across its duration, with a one-second status timeline of the initial 32 seconds and selected full-resolution frames.
- Asho reports available servers/connecting near seconds 5–7.
- Requesting Ad remains visible near seconds 8–26. No creative appears.
- The app proceeds to loading/disconnecting its temporary VPN and then shows not connected.
- Later, the user imports 37 configs in V2rayNG and opens BrowserLeaks successfully.
- No SDK error code, response info, ad request waterfall, or identity of the initial temporary tunnel is visible. Do not equate the later BrowserLeaks test to successful ad playback.

## Fresh upstream evidence
The previously configured ProxyRack endpoint is no longer the active configuration.
Four enabled records named Test were created at 09:44:19–09:44:23 UTC. They have the same endpoint and username; secret comparison in memory confirmed exactly ONE distinct endpoint+credentials combination. No secret or credential digest was printed.
- All 38 currently visible panels selected proxy 24466272-31ec-4112-9936-d95f3f032545 at capture.
- Monitor full HTTPS probe durations were 3.1–5.1 seconds, and later approximately 4.4–4.5 seconds.
- Healthy currently means connectivity succeeded. It is not an assertion of low latency or sufficient throughput.
- Four duplicate records do not provide four independent capacity paths.
- Do not attribute present traffic to the old ProxyRack plan or use historical capacity figures.

## Bounded real traffic tests
Two sequential rounds through an actual Residential VLESS Reality connection from the control server:
| Target | HTTP | Total seconds, round 1 / 2 |
| --- | --- | --- |
| www.google.com/generate_204 (direct control) | 204 | 0.069 / 0.074 |
| adservice.google.com root | 404 | 2.861 / 2.710 |
| pagead2.googlesyndication.com/pagead/js/adsbygoogle.js | 200 | 3.653 / 4.091 |
| browserleaks.com/ip | 200 | 3.199 / 4.458 |

The JS response was approximately 206 KB. Ads TLS setup took approximately 1.8–2.1 seconds. BrowserLeaks reported a non-server exit in both rounds. A 404 at the adservice root is transport evidence only, not an ad-delivery test.

The same selected upstream was then tested directly from the control server using SOCKS5 hostname resolution, bypassing Reality:
- Google control: 1.266 / 2.103 seconds.
- Ad JS: 10.604 seconds for a complete response; next transfer still incomplete at the 12-second deadline (167 KB received).
- Ad root: one failed connection, followed by a 404 response in 2.336 seconds.
- A final bounded check reproduced curl exit 35 for the ad root, while JS completed in 3.379 seconds.

These are small sequential diagnostic samples, not a capacity benchmark. The raw-upstream and Reality measurements used different DNS paths and times, so do not subtract their timings to compute Reality overhead. They independently demonstrate that the current upstream path can be slow and unstable without Reality involved.

## Routing and server checks
Fresh routeTest calls for the current Residential identity:
- pagead2.googlesyndication.com -> intended residential outbound, correct, 11 ms.
- www.google.com -> intended direct outbound, correct, 10 ms.
- browserleaks.com -> intended residential outbound, correct, 11 ms.

These route-test durations measure the API proof, not per-packet routing cost.
Panel runtime: Xray running, version 26.9.9; CPU about 58%, one core; memory about 1.52/2.05 GB; host TCP count 13928, UDP count 2518. Host socket counts are not the number of ad viewers and do not establish Residential UDP use.

Final target routing state remained APPLIED, 38 panels had visible Output snapshots, and both profiles remained enabled with lifetime 0 and creation interval 60 seconds. Fleet-wide routing acceptance was not repeated or declared complete.

## What is established / what remains
Established: current Residential upstream latency and intermittent transfer failure; four identical records; Ads and BrowserLeaks routed correctly on the measured panel; normal TCP control remains direct and fast.

Unresolved:
- Provider/product behind Test is not identified in the saved endpoint metadata, so do not guess its session, bandwidth or concurrency semantics.
- Actual Asho SDK timeout/no-fill/network error and initial tunnel identity require app telemetry.
- Residential UDP remains deliberately blocked; any SDK DNS/QUIC fallback delay is a hypothesis, not established by this test.
- No improvement is claimed and no speculative tuning or direct fallback was introduced.

Next: identify the current upstream provider, inspect its official endpoint/session/limit contract, and compare distinct authorized endpoints with bounded latency/transfer tests. App telemetry is needed to separate transport delay from ad SDK failure.

Private on-server evidence: /root/backups/dob-ad-latency-20261005/{timing.log,rawtiming.log,rawcheck.log,proof.log}. These contain selected metrics only; credential files were ephemeral.
