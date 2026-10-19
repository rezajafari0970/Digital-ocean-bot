# Google Ads selective Residential routing — 19 October 2026

Operator request: allow ordinary phone traffic through the server, while only the Google Ads category uses Residential egress.

## Routing contract

- Authenticated Residential clients: `geosite:google@ads` uses the existing Residential pool; unmatched TCP and UDP use server egress.
- The prior BrowserLeaks diagnostic exception, expanded ad categories, YouTube, Firebase, FCM, Play and root-domain additions are removed from the Residential destination list. A host that is independently in `google@ads` still follows that category.
- DNS uses the existing managed server resolver. Existing destination-IP protections, inbound ownership, TLS verification and dedicated Direct client identities remain in force.
- A Residential outage blocks matching Ads traffic without sending it through Direct. Ordinary traffic and verified selective Output links remain available.
- Internal pool and managed relay inlets keep exact category restrictions and terminal denial. The relay observer uses a HEAD request for a static Google Ads script instead of the removed BrowserLeaks exception; it executes no ad or impression.
- v2rayNG and its core are unchanged. Server routing only sees traffic delivered to the tunnel and names available in the destination or supported sniffing; it cannot identify every encrypted or IP-only advertising flow.

## Staging and rollback

`DOB_RESIDENTIAL_GOOGLE_ADS_SELECTIVE_PANELS` accepts a comma-separated UUID list or `all`. Empty, `none` and malformed scopes preserve the prior policy. The new policy overrides the old allowlist and expanded-category scopes only for selected panels. `all` covers existing and newly created panels on that controller.

Migration 188 adds `panel_routing_state.google_ads_selective`, default false. The reconciler records the mode with the complete pending plan, then the existing native verification promotes that plan. Output's routing revision, fresh verification and client ownership gates remain mandatory; only the proxy-health publication gate is conditional on the verified selective mode.

Rollback sets the scope to `none` (or restores the saved scoped value) and reconciles with the same release. Exact template restoration is covered by tests. Do not manually change client classes, credentials, routing health or publication proofs to simulate acceptance.

## Verification

Required checks include scope/rollback, native TCP and UDP separation, removal of expanded destinations, unchanged Direct clients, sniffed Ads routing, Residential outage without Direct fallback, and Output availability only for fresh verified selective plans. Live rollout receipts and final status are recorded after production verification; this source document does not itself claim deployment or an ad load-rate improvement.


## Production rollout result

Source `3910db010b83163ef0799a488f6b7e7d6933cece` was published as release `8c488709ba27d12d99772636e302d93c0973863b` and installed on both controllers with schema `000188_google_ads_selective`. Both panel workers have the selective scope set to `all`, including future panels. Both controllers passed service, binary identity and all five readiness checks. All 228 retained live listener identities and credentials matched the pre-deployment baseline. Existing policies were preserved; only the normal profile scheduling timestamp advanced.

The acceptance gates passed: 44 mandatory regressions with zero skips against disposable PostgreSQL and race detection; full release validation; 124 independent native canary route proofs; and six real HTTPS connections through the canary's existing Direct and Residential identities across all three profiles, with TLS certificate validation. A first diagnostic omitted a production helper and was invalid; its receipt and correction are retained and were not treated as a network result.

At 13:06:52 UTC (16:36 Tehran), all 77 eligible servers had fresh native verification of the new policy. A subsequent revision change temporarily withheld all Output links while plans were reverified. At 13:11:51 UTC all 77 were freshly verified at revision 690. At the closing observation, 13:15:21 UTC (16:45 Tehran), all 76 still-eligible servers were freshly verified; the difference was one normal expiry at 13:12:32 UTC, not a failed rollout.

Output at 13:15:18 UTC contained 454 of 456 expected links: Trojan 500 was 152/152, VLESS 8443 was 152/152, and VLESS 443 was 150/152. The Output request returned HTTP 200 in 54 ms. These are point-in-time measurements, not an availability or ad-load guarantee.

One independent VLESS TCP REALITY 443 listener remains blocked by `profile_runtime_traffic_unverified`. Bounded diagnostics on that server found its configured REALITY target (`www.microsoft.com`, manual SNI) unreachable while the profile TCP endpoint and ordinary TLS egress succeeded. Both client classes timed out through that listener; the other two profiles on the panel remain verified. This evidence supports a target reachability problem but does not establish its deeper network cause. Automatic retry remains enabled. The two links stay withheld; neither false success nor changed credentials/SNI was used to hide the failure.

The routing change is complete. The residual listener problem and temporary Output withholding mean the entire pre-existing configuration system is not reported as fully fixed. No ad fill-rate, 99 percent load improvement or end-to-end phone speed improvement was measured.

Machine-readable evidence summary: [google-ads-selective-20261019.json](development-jobs/google-ads-selective-20261019.json).
