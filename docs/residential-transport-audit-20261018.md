# Server-side Residential tuning continuation — 18 October 2026

Result: no new permanent performance setting was accepted. The weighted candidate was applied to one server, measured, rejected and exactly restored. Reliable fast Residential transfer and full Config/Output acceptance remain open. No production source or installed runtime changed; external OpenAI API calls: zero.

## Diagnosis and selection evidence

The sampled native runtime is Xray 26.10.10 / dob-fm-0.2.3. Linux already used BBR, receive-buffer autotuning and 32 MiB maximum TCP receive/send buffers. Those settings were not toggled or advertised as a new improvement.

The actual local balancer initially selected the same three available endpoints in its fast and exploration sets. Three separate full AdsJS test rounds across all 12 Residential outbounds produced 8 completed downloads out of 36. Only one endpoint completed 3/3; another completed 2/3. A previously successful endpoint subsequently failed all three rounds. Direct diagnostic controls completed 3/3 in 0.212–0.220 s. These results demonstrate variation in the currently available paths, not a stable fast pool or a measured ad show-rate.

A separate four-pair diagnostic compared inherited destination handling with ForceIPv4 in a temporary process. It recorded 0/4 versus 1/4 completed transfers. This small sample does not justify a DNS rollout; no production DNS setting changed and wire-level destination resolution was not independently instrumented.

## Limited weighted trial

The configuration and scoring rule were frozen before the fresh baseline: 3/3 full downloads maps to cost 0.25, 2/3 to 1, 1/3 to 2 and 0/3 to 4. Candidate fast_count 1, fast_share 90%, health timeout/maxRTT 5 s, interval 10 s; all 12 endpoints remained available to local health-based selection. TCP socket options, routing strategy and buffer changes were disabled. No endpoint was globally disabled, imported or quarantined.

The ten-minute trial started 18:51:01 UTC. Generation 3 was natively verified 18:51:15 UTC. A subsequent native API observation showed that the fast group actually selected the endpoint with 3/3 qualification. All 12 native weight entries were independently verified by canonical cost digests.

| Role | Fresh baseline complete | Valid candidate comparison complete |
| --- | ---: | ---: |
| Trial server | 0/12 | 4/12 |
| Unchanged control | 0/12 | 3/12 |

The initial candidate collection exceeded the diagnostic SSH client's 4096-byte output budget after repeated weight entries were added. Four trial-server transfer outcomes from that window are unavailable. Its original invalid record and the control's valid outcomes remain preserved. The collector was changed to emit weight count/digest and then ran three new complete windows with the same targets, limits and configuration. These are explicitly separate measurements; the invalid window is not counted as successful.

Acceptance required at least 11/12 completions, no degradation, and additional Direct/deny checks before promotion. 4/12 failed the first gate. The unchanged control also improved, so the observed increase is not attributed solely to the candidate. No later promotion checks or fleet publication were performed.

Rollback was requested 18:55:29 UTC. At 18:56:46 UTC the experiment was ROLLED_BACK, target RESTORED, config null, generation/applied_generation 4, and the original native plan hash 040014bf3326f99ec40d7be8672eac5ce1000f745dcc793d929ea8a870b19f32 was restored. The final inventory independently confirms no active experiment or assigned performance configuration.

## Provider and timeout investigation

Direct SOCKS-stage diagnostics on the control node showed authentication and CONNECT success on all 12 endpoints. Ten returned verified TLS plus HTTP 200 headers. One timed out during TLS and one after TLS while waiting for HTTP. Header-level connectivity does not establish full-body transfer speed or long-term reliability.

The configured ProxyRack account reported one active Taiwan sticky session, 15 available sticky-session slots and a 2100 thread limit. Its existing session's idle timeout was 15 s and TTL 600 s. The provider supports a separate named session and a per-authentication timeout override.

Two isolated diagnostic sessions were created sequentially within the existing subscription; each was released, and the original session identity was verified unchanged. No production user traffic was assigned to them. The first probe stopped before asset testing because the API returns a port-prefixed session ID; the helper had expected a bare name. Its initial absent=true field was therefore invalid. Follow-up located the exact prefixed ID, released through the immutable diagnostic authentication selector, and confirmed the original session remained.

The corrected second probe independently confirmed country TW, timeout 60 s, TTL 180 s and a distinct online session. Its AdsJS and GPTJS requests both failed TLS establishment (curl 35) after 2.375 s and 0.743 s, respectively. The 60-second setting was accepted by the provider, but these failures occurred much earlier. The diagnostic session was released successfully; the account returned to one session with its original identity and settings.

This evidence does not support deploying a larger timeout as a speed fix or claiming 99% ad loading. It leaves a provider/path/TLS interoperability problem requiring upstream resolution or destination-qualified replacement capacity. It does not establish that every account is saturated, every endpoint is permanently dead, or a specific application SDK is faulty.

## Closing state

Primary checks at 19:02:29 UTC passed exact live binaries, three active services, five readiness samples, zero unexpected restarts and environment/policy/credential/native-identity preservation. The secondary passed exact live binaries, all three services, zero restarts and readiness at 19:02:24 UTC. Both remain on release 16f3b682f638fba5021588fe9818b91db917ef66.

At 19:03:06 UTC on 18 October 2026 the current 77-server cohort showed 451/462 published links. Output returned HTTP 200 with 451 lines in 46 ms. All 228 existing native proofs were younger than five minutes; three new listener proofs had not yet been recorded. The fleet denominator changes through ordinary server lifecycle, so this is not a direct speed comparison with an earlier cohort.

| Stage | Status |
| --- | --- |
| Read current runtime and kernel settings | Completed |
| Repeated individual-path qualification | Completed; no stable high-success pool established |
| Weighted selection trial | Applied and measured; acceptance failed |
| Exact native restoration | Passed |
| DNS comparison | Inconclusive; not deployed |
| Separate provider session and 60 s timeout test | Completed; TLS failures persisted; sessions cleaned up |
| Operational/credential preservation | Passed |
| Reliable fast Residential transfer and complete Output | Open |
| 99% actual ad load/show target | Unverified |

The prior 41 mandatory, 11 mobile and 6 Output tests apply historically to the unchanged release and were not rerun. Full raw safe observations and evidence digests are in docs/development-jobs/residential-transport-20261018.json.

Official provider references:

- https://help.proxyrack.com/en/articles/5829891-set-connection-timeout
- https://help.proxyrack.com/en/articles/5977249-create-multiple-sessions-on-a-sticky-port
- https://help.proxyrack.com/en/articles/5829865-set-the-rotation-interval-for-a-sticky-session-port
