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
