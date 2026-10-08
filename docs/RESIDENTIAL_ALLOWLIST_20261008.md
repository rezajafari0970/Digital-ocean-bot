# Residential destination allowlist — 2026-10-08

User-authorized change: RESIDENTIAL subscriptions may reach only the existing Google advertising category, the already enabled BrowserLeaks diagnostic scope, and exact configured probe hosts. Previously all other destinations escaped through the server-direct fallback. This replaces that fallback with blackhole.

## Policy

- Category: `geosite:google@ads`; diagnostic: `domain:browserleaks.com`.
- Probe hosts: `full:www.gstatic.com`, `full:connectivitycheck.gstatic.com`, TCP ports 80/443, through the residential egress/pool.
- Unmatched domains, raw/opaque IP requests, client DNS port 53, other probe ports and probe UDP are blocked. Category UDP retains the existing SOCKS-only capability gate.
- The pool's loopback second stage checks the same allowlist and denies unmatched targets. Missing/unhealthy upstream cannot become direct fallback.
- Explicit DIRECT identities retain their independent direct routes. Unknown identities are protected. Management and internal Xray DNS remain infrastructure, not subscription destinations.
- Internal DNS routing is restricted to the configured resolver IPs (currently 1.1.1.1 and 8.8.8.8), TCP port 53 or 443 according to the existing TCP/DoH performance setting.
- Strict inbound sniffing requires hostname destination override. `routeOnly:true`, disabled/incomplete sniffing or exclusions cause a blocking residential plan, not a silent bypass.

## Rollout and rollback

`DOB_RESIDENTIAL_ALLOWLIST_PANELS` is a deployment-only selector: unset/`all` enables current and future panels; a comma-separated validated UUID list enables a canary; explicit `none` restores the historical selective policy. UUID matching is case-insensitive. A malformed selector fails strict. Native acceptance uses this same expected selector; it cannot certify legacy routing as a successful strict rollout.

Use the shared deployment lock and recheck canonical/runtime before publishing. Concurrent account/panel repair work is in a different checkout and must not be overwritten. Preserve credentials, client lifetimes/quotas, mutation gates, account policies, journals and pending recovery state. Keep the retired browser manager disabled.

Rollback of the routing policy: set the selector to `none` on the panels worker, restart that role, then verify the legacy policy with the same explicit selector. Keep the reviewed code and ledger data. Rollback changes the residential policy back to direct fallback and therefore must be an intentional operator decision, not an automatic health response.

## Verification and limits

Unit tests cover normal/missing/failed upstreams, invalid sniffing, stale route-class metadata, unassigned users, exact probe host/port matching, DNS boundaries, pool second-stage denial, DoH compatibility, stable fingerprints and legacy rollback.

Isolated tests use actual Xray, authenticated VLESS users, TCP/UDP responses, upstream outage/recovery, and independent DIRECT positive controls. The 26.9.9 fixture explicitly allows local test sinks because that version's Freedom default blocks private addresses. The UDP fixtures use authenticated VLESS requests, retaining domain destinations instead of SOCKS client-side DNS conversion. These fixture changes do not weaken production egress guards. No persistent probe service is installed.

This is destination access control inside authenticated proxy sessions. It cannot stop incoming Internet SYNs or unrelated host traffic. HTTPS URL paths and response content remain encrypted: an allowed probe host is not limited to `/generate_204`, and an allowed site could provide other content or tunneling. It is not an absolute content filter.

Client DNS sent through this restricted subscription is blocked. Clients must resolve locally or supply domain destinations; arbitrary DoH/DNS hosts are not silently added to the allowlist. A mobile compatibility test remains distinct from native routing proof.

Deployment and native fleet results are recorded in `RESIDENTIAL_ALLOWLIST_ACCEPTANCE_20261008.json`. Unreachable/expired panels are not counted as successfully changed merely because the code is deployed.
