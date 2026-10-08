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

## Observed rollout result and exact continuation

Strict routing deployed from `7cc709e` after integration with account/panel repair `a41047c`; all current/future panels select the strict policy (`DOB_RESIDENTIAL_ALLOWLIST_PANELS=all`). A replacement canary was used because the original was deleted. Independent native acceptance converged to **22/22 serving panels**, at 2026-10-08T15:43:23Z. The exact panel IDs and timestamps are in the acceptance JSON. The final inventory also contains **73 expired/retiring/deleting panels** that are excluded from this serving proof. Do not call those verified.

Actual VLESS/Reality traffic with installed Xray26.3.27 passed **9/9**: DIRECT positive controls, both probes(204), BrowserLeaks(200), advertising host(404 response proves connection), and blocked ordinary domain/rawIP/arbitraryDoH. Earlier errors and diagnostics remain in the evidence directory. Default client26.9.9 failed some TLS traffic; its TLS1.2 test passed the positive destinations. This is a remaining compatibility observation, not a proven root-cause fix or mobile acceptance.

Four distant panels still show periodic FAILED/APPLYING despite independent correct native routes. The controller caps its whole sequential proof at8seconds. A 300ms-per-request test reproduces the failure on the deployed source; extending the aggregate budget to30seconds passes while retaining the caller45second deadline and all mutation/readback guards. Follow-up commit **3fad47751e15697b2310be19884658e31606b6b0**, branch `fix/residential-proof-timeout-20261008`, contains only that bounded timeout adjustment and regression test. Full Go regression and relevant race tests pass. It is **not deployed**.

Independent follow-up review is **blocked**: the existing OpenAI API returns HTTP429, `insufficient_quota` / `credit_balance_exhausted`, without a retry time. Do not keep retrying on a timer or claim review PASS. Restore review credit or obtain independent human review; then rebase the pending fix onto the latest canonical HEAD, revalidate source hashes and runtime, and guarded-deploy it. The prepared deployment helper must have its canonical/runtime expectations refreshed because the canonical report commit and running code commit differ. Verify periodic APPLIED state on all serving panels afterward. Main feature rollback runtime is `/opt/digital-ocean-bot.rollback.DzXr1KL8`; policy rollback deliberately restores legacy direct fallback and is not an automatic response to this status problem.

## Progress reporting preference

The user requests visible, accurate progress. During active work, report the current action, observed result and any wait/blocker at least every60seconds. Persist timestamped status and next action in the acceptance JSON. On yielding, explicitly distinguish agent work ending from the application's normal workers continuing. Do not leave an IN_PROGRESS status implying an unattended agent is still working.
