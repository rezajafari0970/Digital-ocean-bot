# Fleet repair follow-up — 2026-10-17 Tehran

Recorded at 2026-10-17T03:35:41.659061+03:30. Continued from checkpoint 4653a9c; product source and installed public revision remained unchanged. Both service hosts run public 2471b22a90dead5468d3ed1a2acc9dc7d16a0f71. All six service processes are active with matching executable hashes and zero automatic restarts; all readiness checks passed.

## Live output verification

The initial window had 5 / 6 samples with full healthy-server coverage, saved profile targets and zero invalid links across authenticated output and existing public shares. The last initial sample observed a newly READY panel about six seconds after registration: it had not yet entered the first output snapshot. This startup gap is retained in the evidence. The existing bounded capacity reconciler then completed its saved profiles without changing execution gates, cadence, operation budgets or policy.

All three terminal samples between 2026-10-17T00:04:26.425531+00:00 and 2026-10-17T00:05:07.676196+00:00 had full healthy-host coverage in all six endpoints. The final sample also met every saved profile target; the earlier in-flight creation observation is retained. Last healthy cohort: 93. Last health reasons: {"HEALTHY": 93}.

| Output | Servers | Links | Missing healthy servers | Invalid links |
|---|---:|---:|---:|---:|
| DIRECT | 93 | 93 | 0 | 0 |
| RESIDENTIAL | 93 | 279 | 0 | 0 |
| ALL | 93 | 465 | 0 | 0 |
| SHARE_DIRECT | 93 | 93 | 0 | 0 |
| SHARE_RESIDENTIAL | 93 | 279 | 0 | 0 |
| SHARE_ALL | 93 | 465 | 0 | 0 |

## Native runtime and remaining barriers

The read-only native audit verified 92 / 92 current eligible panels against architecture-specific on-disk and running-image hashes. All had bounded fallback, a handshake deadline and fallback concurrency limit 512. 31 panels were new since the previous acceptance audit and already had the corrected core. The later arrival passed the same checks, bringing this follow-up to 93 distinct verified panels. There were no active transport barriers, pending/running/failed client jobs on ready panels, current provider errors or verified admission blocks in the final barrier snapshot.

Resource follow-up covered 1 still-live original canary. It had 10,712 seconds of uninterrupted corrected-core uptime, 1,453,184 KiB available memory, zero memory PSI avg10/avg60/avg300, and a successful 37 ms statistics response. Deleted historical canaries were excluded by deployment identity, including the reused host address of the new arrival.

## Result and continuation

The previous repairs remained effective. The standard capacity reconciler was used for one newly ready panel; no additional source change, deployment, policy toggle, provider deletion or direct native/client-table write was needed. Product tests and publication gates accepted in the preceding repair were not repeated. No health state or freshness rule was suppressed to display zero. Model API requests, tokens and cost for this continuation are all zero.

Exact per-sample data and operational receipts: `docs/development-jobs/route-pressure-20261017-followup.json`. Recheck live state from this checkpoint if a new discrepancy appears; current counts are not a permanent availability guarantee.

A transient provider-network error was observed during terminal sampling and recovered through the existing automatic path. The final provider state was ACTIVE / READY with no error, and the final dashboard had no provider-unavailable servers. Both the transient observations and the later recovery timestamp are retained in the companion JSON; no error flags were manually cleared.
