# Residential transport continuation — 18 October 2026

Status: candidate rejected and restored; slow/failing Residential egress and complete Output acceptance remain open. No production source change or runtime deployment was made. External OpenAI API calls: zero.

## Controlled trial

The predeclared plan was saved at18:23:25 UTC. A ten-minute, one-server trial started at18:25:44 UTC with a separate unchanged control server. Three windows each contained four static JavaScript transfers: sequential AdsJS/GPTJS and the same two resources concurrently. TLS verification stayed enabled; no scripts, ad requests, impressions or clicks were executed, and there were no retries or redirects. Each transfer had an8-second total measurement deadline and4-second connection deadline. These are collector limits, not the application SDK or production request timeout.

The candidate changed fast selection from4/12 to2/12, fast share80% to90%, and local health/maxRTT thresholds3s to5s. TCP options were disabled, so the configured but inactive tcp_user_timeout value did not change sockets. DNS, buffering, independent Direct/Residential policy and strict fail-closed behavior were preserved.

| Role | Baseline complete | Candidate complete |
| --- | ---: | ---: |
| Trial server | 2/12 | 2/12 |
| Unchanged control | 1/12 | 3/12 |

Acceptance required at least11/12 completed transfers and no degradation, with additional Direct/deny controls before promotion. The completion gate failed. The candidate was rejected at18:28:35 UTC and explicitly rolled back. At18:29:27 UTC, experiment ROLLED_BACK, target RESTORED, null performance configuration, generation/applied_generation2 and the exact original native plan hash were independently verified. No fleet publication took place.

## Separate diagnostics after restoration

A30-second, uncompressed diagnostic completed GPTJS on the trial server in3.276s and on the control in29.964s. AdsJS remained incomplete on both at30s despite HTTP200 and partial body delivery. A longer deadline can allow a slow transfer to finish; this evidence does not demonstrate faster display or justify changing SDK timeouts.

An additional eight transfers requested curl-supported compression. None completed within8s. Negotiated content encoding was not recorded, so these observations are not presented as proof that gzip was actually delivered.

A read-only provider API request through the configured ProxyRack domain returned149 and later233 open connections against threadLimit2100. The second response reported bandwidth allowed3750000 and usage41631 in the provider's raw units. This is a point-in-time observation for the responding gateway/account, not proof about every endpoint or a measured bandwidth speedup. Two IP-address gateways did not answer this provider API within5s; lack of support for that API is not itself proof of broken proxy service.

The per-egress static AdsJS test then bypassed native pool selection in a temporary loopback diagnostic process while retaining existing proxy authentication in memory. One of12 Residential paths completed within8s (3.685s); ten timed out and one failed TLS establishment. The independent Direct diagnostic completed in0.215s. These samples support an egress bottleneck and show why a generic connectivity success cannot stand in for full asset delivery. They are a single observation per endpoint, not eligible repeated-failure evidence for admission or fleet quarantine. Production advertising was not rerouted to Direct.

The first per-egress collector run produced an unparseable, truncated SSH result; it is retained as invalid evidence. A separate new run used a compact structured response and supplied the recorded observations. No failed outcome was replaced with a successful retry. An earlier local helper variable collision affected receipt placement; the original request/receipt were located and moved without replaying the mutation. Both issues were confined to the diagnostic harness.

## Closing operational checks

Primary verification at18:37:50 UTC passed exact binaries, all three services, five readiness samples, zero unexpected restarts and existing environment/policy/credential/native-identity preservation checks. The secondary independently passed exact binaries, all three services, zero restarts and readiness at18:37:44 UTC. Both remain on release16f3b682f638fba5021588fe9818b91db917ef66, schema187 on primary.

At18:37:01 UTC,72 eligible servers had425 published links out of432. Output returnedHTTP200 with425 lines in42ms; all216 eligible native proofs were under five minutes old. Seven links remained withheld. This is not complete acceptance; catalog and Output were consecutive separate reads. The live fleet denominator differs from the earlier75-server checkpoint.

| Stage | Result |
| --- | --- |
| Baseline and predeclared comparison | Completed |
| Limited tuning | Applied, measured, rejected |
| Exact native restoration | Passed |
| Provider/individual path diagnostics | Completed with stated limits |
| Runtime and policy preservation | Passed |
| Complete Output / reliable fast Residential transfer | Open |
| Actual app ad load/show target of99% | Unmeasured; app SDK source and callbacks needed |

Further action requires viable destination-tested Residential paths and, for real ad display optimization, the application repository/SDK version plus load/show/error timing traces. The current evidence does not justify removing most endpoints, changing identities, weakening TLS, or claiming a99% result. Historical41 mandatory regressions,11 mobile scenarios and6 Output cases belong to the unchanged deployed release; they were not rerun in this continuation.

Machine-readable observations and evidence digests: docs/development-jobs/residential-speed-20261018.json.

Provider API references:
- https://help.proxyrack.com/en/articles/5829914-get-current-thread-usage
- https://help.proxyrack.com/en/articles/5741065-is-there-any-thread-simultaneous-connection-limit-for-residential-proxies


## Later server-only continuation

See [the weighted-selection and provider-timeout continuation](residential-transport-audit-20261018.md). The later trial also failed acceptance and was fully restored; both temporary provider sessions were cleaned up. Latest output checkpoint and detailed limits are in that report.
