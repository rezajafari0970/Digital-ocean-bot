# DNS and proxy-path resilience — 2026-10-26

## Problem and scope

Independent direct/raw-SOCKS/native measurements showed intermittent UDP53 loss on the only measured UDP-capable external proxy, while QUIC443 still answered. The previous generic UDP gate correctly required DNS plus QUIC for broad UDP, but also applied that requirement to category-only QUIC. This unnecessarily withdrew working UDP443 paths when DNS failed.

The opt-in deployment scope is `DOB_DNS_RESILIENCE_PANELS` (panel UUIDs, `all`, or `none`). Empty/malformed scopes retain the previous plan. Existing account, client identity, TLS, expiry, ownership, and protected-routing policies remain authoritative.

## Implementation

- Six unfiltered IPv4 DNS endpoints: Cloudflare 1.1.1.1/1.0.0.1; Google 8.8.8.8/8.8.4.4; AdGuard nonfiltering 94.140.14.140/94.140.14.141. These are six endpoints across three independent providers.
- Scoped native DNS uses routed TCP, per-server 2000ms deadlines, parallel queries and positive caching. No `+local` or system-resolver bypass is added. Category-only DNS retains its existing authorized direct egress; full-tunnel DNS retains its configured proxy route.
- Existing explicit DoH performance profiles retain their configured HTTPS endpoints. This change does not silently replace an explicitly selected transport.
- Persisted observations and their schema keep the original DNS/QUIC semantics. Fresh QUIC proof is hydrated separately, bound to exact program/transport/credentials and origin generation, with the existing ten-minute TTL and future-time rejection.
- Partial proof may select a proxy only for category UDP443. Public category, donor, and pool-loopback rules reject other UDP ports before forwarding. Full-tunnel UDP still requires the full existing proof. TCP selection stays independent and protected traffic never falls back direct.
- Live native route admission now verifies UDP8443 alongside UDP443; pool outcome verification rejects UDP-ineligible members.
- Scope removal restores the legacy resolver/routing plan without credential or client changes.

## Validation and limits

Initial owned-canary baseline on 2026-10-26: 36/36 valid DNS TCP/UDP answers, three samples per endpoint and transport; production configuration unchanged. This is a bounded sample, not a perpetual availability guarantee.

Targeted Go suites passed. Pinned-core tests passed with five hanging DNS fixtures and the sixth answering, positive-cache reuse, all-upstream failure, QUIC-only managed donor acceptance on UDP443 and rejection on UDP8443. The canonical isolated gate requires 250 test roots, including real PostgreSQL, native transport and race checks. Full CI and production rollout are recorded separately; they are pending at this checkpoint.

If all DNS endpoints fail for an uncached name, the pinned core may drop the request instead of synthesizing SERVFAIL. The final visible timeout is controlled by the client (five seconds in the fixture), not a promised two-second end-to-end SLA. No server-only DNS change can create a second genuinely working UDP upstream or establish mobile ISP/HTTP3 throughput.

## API review accounting

The user explicitly requested a new strongest-model review. The new `--independent-review` mode authorizes a distinct prompt/label while preserving every prior unknown reservation at full worst-case cost, duplicate fences, pending-receipt protection, the same $2 aggregate cap and three-request maximum. Default unknown-outcome behavior is unchanged. It never retries or releases the legacy request.

The new gpt-6-astra / max background request has a saved receipt. Total conservative reserved amount is $1.920628 including the unchanged $1.501011 legacy reservation; actual usage and review findings are pending. All 28 offline API accounting/recovery tests passed. Secrets are not part of the review prompt.

## Sources

- https://xtls.github.io/en/config/dns.html — routed TCP, per-server timeout and parallel query semantics.
- https://developers.cloudflare.com/1.1.1.1/ip-addresses/ — Cloudflare resolver addresses.
- https://developers.google.com/speed/public-dns/docs/using — Google resolver addresses.
- https://adguard-dns.io/en/public-dns.html — nonfiltering AdGuard endpoints.
- https://developers.openai.com/api/docs/models/gpt-6-astra — model and reasoning support.
