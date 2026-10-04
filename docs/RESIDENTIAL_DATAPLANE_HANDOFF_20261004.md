# Residential data plane and creation interval — 2026-10-04

Runtime: `8425e1bebd4aff0b0e3031615ac0d646ecd7ae44`. Canonical continuation branch unchanged. Migration 149 applied. API/worker active; main gate restored enabled, concurrency 1. Old bulk gate remains closed. No new mutation failures.

## Observed problems and fixes

The supplied 107-second video was reviewed, including the phone's detailed Xray logs. DNS failure was visible before the selected 4-minute credential expired. Real tunnel tests reproduced intermittent Residential UDP DNS failure while Direct worked. Ordinary recognized domains exited with the server IP: traffic was not universally Residential.

Only Cloudflare 1.1.1.1, Google 8.8.8.8 and AliDNS 223.5.5.5 remain in the managed resolver pool. Residential client UDP/TCP DNS is intercepted and carried upstream over protected TCP. A/AAAA use the three-provider pool; other query types use a TCP DNS outbound chained to the same Residential egress. No direct DNS fallback is allowed.

Installed versions differ: control Xray26.3.27; target Xray26.9.9. The first TCP-DNS candidate used removed proxySettings and was rejected by target panels. It was rolled back. The final version uses streamSettings.sockopt.dialerProxy and was validated with the actual target binary. Exact target-source commit52a412d confirms legacy DNS network/address/port aliases remain supported; nonIPQuery is currently accepted with a deprecation warning. Do not replace this with unverified latest-doc syntax.

Creation interval is now stored per independent profile and enforced by a durable per-panel/inbound/class clock in the planning transaction. 6 minutes maps to 360 seconds. It limits creation while below target; a full target does not rotate by itself. To rotate one shared credential every approximately 6 minutes, lifetime and interval both need appropriate settings. Creation rate does not measure connected users. Existing production policy was preserved: Direct lifetime0, Residential lifetime240 seconds, both intervals0.

## Acceptance and limitations

Actual data-plane tests passed ordinary HTTPS, Google, BrowserLeaks (Residential non-server IP), advertising HTTPS, and DNS A/AAAA/HTTPS records over both UDP and TCP client inputs. TLS sniffing was also exercised when SOCKS supplied an IP destination. All38 panels passed the new saved-DNS and running-route checks over the acceptance window; one panel updating concurrently was independently re-read. This is not a claim that all38 were simultaneously fresh/APPLIED.

The real-core fault test rejects upstream UDP, proves DNS still works over TCP, then disconnects the proxy and verifies zero direct-DNS sink connections. Focused, race, full, clean-build and mobile UI tests passed.

Residential HTTP3 failed during an unexpired credential while Direct succeeded. A prior baseline Residential HTTP3 had passed. DNS is corrected, but reliable general UDP/QUIC and large concurrent-user capacity remain unaccepted. Do not call the whole Residential system stable. Proxy health flapping also changes the selected endpoint and can temporarily withhold Output during routing reconciliation.

Credentials copied as static URIs still expire at their configured absolute expiry. One attempted test crossed this deadline and was discarded as transport evidence; the final DNS acceptance guarded credential age. Tests must check both Output freshness and remaining credential lifetime.

Evidence: `/root/backups/dob-residential-dataplane-20261004`, especially `v3/fresh-sniff.log`, `v3/fresh-quic.log`, `v3/fleet-accepted.log`, `v3/fleet-accepted-recheck.log`, and `v3/ui.log`. Private candidate settings and raw URIs must not be printed.

Next: diagnose Residential UDP/QUIC with provider-neutral per-endpoint capability and capacity measurements. Do not bypass fail-closed routing, reuse account proxies as Residential, relax freshness, or promise 30k concurrent connections without a real load test.
