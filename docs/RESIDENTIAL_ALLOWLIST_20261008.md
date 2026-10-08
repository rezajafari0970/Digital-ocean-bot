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

## Verified continuation — 2026-10-09 Tehran

API credit was restored and independent review passed: `resp_05bc3d42a32fa1b5006ac8179189d087d19ed1c24cb7f1c6a4`, no blockers. The previously pending verifier fix was integrated onto canonical5e6c10d as **d02f0450e2e8b71e5ff4b28ed52cb03eefe2f59e** and guarded-deployed at **2026-10-08T22:26:27Z**. The original branch/commit3fad477 remains historical; no review gate was bypassed.

The confirmed defect was the8-second aggregate deadline for sequential route proofs. A delayed-request regression fails on old code and passes after the deadline becomes30seconds, inside the existing45-second operation deadline. Full Go regression and focused race checks pass on the integrated source. One build failed only because the `/tmp` tmpfs was full; a root-disk temporary directory resolved the build environment failure. No user files were deleted.

Initial native proof passed28/28 and three consecutive periodic samples were28/28 APPLIED. Two newly created panels were then independently checked. At **2026-10-08T22:39:27Z / 2026-10-09T02:09:27+03:30**, all **30/30 current serving panels** were native-verified and APPLIED. API/control/panels roles are active, NRestarts0, exact running binary hashes match the clean manifest, all five readiness checks pass, and the retired browser manager stays inactive/disabled.74 expired/retiring/deleting records are excluded, along with1631 separately classified historical DELETED records.

Actual traffic reused the same credentials across versions on a comparison canary: installed26.3.27 first8/9, target26.9.9 **9/9**, installed26.3.27 repeat **9/9**. The aggregate is **26/27**, not27/27; one initial www.gstatic.com positive probe failed curl35 with no proven specific cause. Both probes, advertising category, BrowserLeaks, explicit DIRECT controls and deny cases passed in each of the last two runs. Older claims that26.9.9 alone caused TLS failures are superseded by this controlled comparison. Mobile/ISP compatibility remains unmeasured.

### Separate capacity finding

Repeated failures on loaded canary `d4fbef39-e05c-464c-a999-72a02ff87c18` were **TCP443 connection refused** on both client versions. Sanaei showed Xray running and a stable APPLIED plan. The existing guardian reportedCPU100%, CPU queue pressure, admission_blocked=true and managed listeners verified. Its owned nft rule intentionally rejects new SYNs with TCP reset during pressure/recovery. A separate bounded watch saw5 open/5 refused connections on that panel versus10/10 open on the comparison panel. Central receipts lagged by up to73seconds in that watch, so exact packet-to-receipt alignment is not claimed.

At the final snapshot, **4/30** serving panels had fresh admission-blocked receipts:3 CPU-critical and1 recovering. This is a remaining capacity/availability issue, not an allowlist escape. Protection, existing sessions, quotas/lifetimes, server sizes/counts and credentials were preserved. No new persistent probe service is running.

### Three phases and next phase

1. Design: complete; destination scope, DNS/sniffing boundaries, fail-closed pool and intentional rollback documented.
2. Implementation/review/tests: complete; reviewed d02f045 is deployed and regression/race evidence retained.
3. Rollout: routing/status acceptance complete30/30 with the capacity and traffic limits above. Do not call all production availability problems solved.

Phase4 is capacity and connection stability: measure guardian admission versus CPU/conntrack/client pressure, design capacity-aware output/failover using fresh verified receipts, and test real mobile clients with local DNS/domain destinations. Client failover is not implemented by this change. Changing provider capacity/spend, disabling protection, or altering user quotas is not an implicit fix. Runtime rollback before the verifier correction: `/opt/digital-ocean-bot.rollback.BZTZp014`.

## Progress reporting preference

The user requests visible, accurate progress. During active work, report the current action, observed result and any wait/blocker at least every60seconds. Persist timestamped status and next action in the acceptance JSON. On yielding, explicitly distinguish agent work ending from the application's normal workers continuing. Do not leave an IN_PROGRESS status implying an unattended agent is still working.
