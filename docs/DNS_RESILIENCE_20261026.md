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

Targeted Go suites passed. Pinned-core tests passed with five hanging DNS fixtures and the sixth answering, positive-cache reuse, all-upstream failure, QUIC-only managed donor acceptance on UDP443 and rejection on UDP8443. The canonical isolated gate passed all 250 required test roots with zero required skips (run 40290161300, source 44754e30233a1c862d3d64b42e8cb0cad13ebe6d), including real PostgreSQL, native transport and race checks. Public release 925c48f0ab02be0016e9f7cce4928480bfcdb801 passed install and test jobs in run 40291465726; promotion run 40291823516 advanced main and stable atomically. Deployment acceptance is recorded separately below.

If all DNS endpoints fail for an uncached name, the pinned core may drop the request instead of synthesizing SERVFAIL. The final visible timeout is controlled by the client (five seconds in the fixture), not a promised two-second end-to-end SLA. No server-only DNS change can create a second genuinely working UDP upstream or establish mobile ISP/HTTP3 throughput.

## API review accounting

The user explicitly requested a new strongest-model review. The new `--independent-review` mode authorizes a distinct prompt/label while preserving every prior unknown reservation at full worst-case cost, duplicate fences, pending-receipt protection, the same $2 aggregate cap and three-request maximum. Default unknown-outcome behavior is unchanged. It never retries or releases the legacy request.

The gpt-6-astra / max request settled as incomplete: its 7168 output tokens were reasoning and no final review text was returned. A distinct, focused request using the same strongest model with low reasoning completed. It approved the scoped strategy, explicitly not deployment readiness, and identified resolver query exposure/inconsistency, stale or overgeneralized QUIC evidence, and public/donor/loopback policy-boundary risks. Native boundary tests and live acceptance remain required. No credentials or user configuration secrets were submitted.

The optional --reasoning-effort argument now records the actual effort in both payload and receipts; the default remains max. All 29 offline accounting/recovery tests pass. The two current requests have conservative usage cost bounds of $0.365797 and $0.017466. Total accounted upper amount is $1.884274 including the unchanged $1.501011 legacy unknown reservation. These are not invoice totals. The three-request maximum is reached; no further generation request is issued. Public runtime export excludes these review helper files.

## Sources

- https://xtls.github.io/en/config/dns.html — routed TCP, per-server timeout and parallel query semantics.
- https://developers.cloudflare.com/1.1.1.1/ip-addresses/ — Cloudflare resolver addresses.
- https://developers.google.com/speed/public-dns/docs/using — Google resolver addresses.
- https://adguard-dns.io/en/public-dns.html — nonfiltering AdGuard endpoints.
- https://developers.openai.com/api/docs/models/gpt-6-astra — model and reasoning support.

## Production acceptance and incident history

Both isolated instances run public release 925c48f0ab02be0016e9f7cce4928480bfcdb801 with schema 000200_protocol_probe_drivers unchanged. Default verification completed at 08:29:04 UTC; production runtime completed at 08:58:28 UTC. Actual panel-worker processes verified DNS scope all at 09:19:01 and 09:22:21 UTC. Six services are active with exact artifact hashes.

- Canary 45.77.185.94: native six-resolver/parallel/cache/port-guard checks passed; all four actual VLESS identities and Flow values matched. Real client DNS 16/16, QUIC version-negotiation 40/40, neutral HTTPS 204 responses 8/8. Temporary clients stayed loopback-only and production configuration files were unchanged by measurement. Expired previous target 79.137.10.153 was not resurrected. These are server-local client tests, not mobile-ISP, full HTTP/3 or throughput measurements.
- Fleet native audit started 09:28:44 UTC: 92/92 nodes passed, all 368 VLESS identities/Flow values matched, and all six TCP DNS endpoints answered on every node (552/552 validated replies). Two nodes had no configured UDP candidate during initial sampling; targeted 09:34:16 UTC follow-up found a verified UDP candidate on both, with 12/12 additional valid DNS replies. Initial evidence was retained.
- Final repeated coverage at 09:33:46 UTC: 92/92 eligible nodes freshly observed, 92/92 routing states APPLIED, no probe execution errors, both instances ready. All 91 remaining members of the original cohort were observed in two cycles; normal expiry/creation explains cohort changes. Fresh positive QUIC coverage was 91/92 in this last snapshot, versus 92/92 in an earlier snapshot. Provider-path variability remains observable; an executed probe is not the same as a positive path result.
- There remains only one demonstrated UDP-capable external proxy among twelve. The new policy prevents unrelated DNS loss from withdrawing a proven category QUIC path, but cannot manufacture a second independent working upstream or eliminate provider packet loss. A missing current proof remains fail-closed.

The first production install stopped before runtime swap at insufficient_retention_workspace. Staged artifacts were retained and reverified. Thirteen closed rotated logs were preserved in two byte-verified compressed archives, reproducible compiler/APT index caches were reclaimed, and database/backups were not manually deleted. Resume reused only sealed artifacts; native fresh backup and disk-space checks stayed intact. Normal native backup retention remained authoritative.

Sixteen preservation checks passed per instance after accounting explicitly for concurrent activity. The user confirmed deleting one AWS account while this task ran: its one schedule and four panel-listener records are recorded exceptions, not claimed unchanged. Three disabled profiles completed deletion requests from the previous day. Production comparison retained 6,919 common profile-owner records with zero unexpected changes (ten expected expired retirements), and all 77 Trojan records unchanged. Secrets, key/environment hashes, active policies and unrelated account settings stayed intact. No deleted user account was recreated.

Rollback: set DOB_DNS_RESILIENCE_PANELS=none in the existing scoped worker drop-ins, restart panel workers, and verify the legacy plan in native cores. The prior 7d2981ccacdd90da8306bbfcb54d1294f33afdfb runtime and verified backups remain available. Do not bypass native integrity dependencies or host-key/ownership fences.

Operational evidence is retained under /root/dob-dns-resilience-20261026 with unique stage receipts, failure history, API usage receipts, native samples and coverage snapshots. Final documentation and API helper changes do not intersect public-export paths; public runtime artifacts remain the exact CI-tested release.
