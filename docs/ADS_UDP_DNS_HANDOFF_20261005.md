# Ads UDP and managed DNS — 2026-10-05

Status: deployed; managed DNS and ordinary UDP/HTTP3 accepted on a real Reality tunnel. Ads remain blocked by upstream AUTH_REJECTED.

Latest user authorization supersedes the earlier blanket Residential UDP denial: enable UDP, keep only Ads plus the temporary BrowserLeaks exception on Residential, and keep ordinary traffic and DNS direct from the VPN server.

Changes: matched Ads TCP/UDP use SOCKS, HTTP protected UDP remains domain-scoped blocked, other UDP is direct. Residential plain DNS on UDP/TCP 53 is intercepted into the cached IPv4 Cloudflare/Google TCP pool, independent of residential endpoint health; non-IP queries are forwarded by the DNS outbound through the managed direct outbound. Existing Direct identities bypass the new DNS interceptor. Encrypted DNS is not intercepted. No arbitrary domain expansion or provider credential mutation.

Current upstream observation: only one enabled record remains, ID ca78ed2f-0649-4d79-8430-f4ac11233445, SOCKS5, status down; last success 2026-10-05T10:22:07Z. The endpoint is not the prior ProxyRack hostname. Native upstream HTTPS tests failed with curl35. UDP/HTTP3 on this endpoint is unconfirmed. Never claim a successful ad display or attribute this endpoint's limits to the old provider.

Output already exports xtls-rprx-vision-udp443. No exporter change was necessary. Asho client source is not part of this working tree; SDK timing and Android Private DNS still require app-side validation.

Evidence: /root/backups/dob-ads-udp-dns-20261005. Focused race, installed-core fault tests, full go test ./..., syntax/diff checks passed. The DNS test verifies UDP/TCP A answers, cache hits, failed-primary fallback, empty AAAA under IPv4 policy, and real forwarded HTTPS65 NXDOMAIN. Non-Ad UDP delivery and matched Ads TCP/UDP fail-closed are tested against local sinks.

First UDP fixture reused a socket/destination and its direct positive control failed; the fixture now uses a separate source socket per destination. Original failure log is retained. No pass was claimed from a failing positive control.

AI OS Development Orchestrator/API plan used; initial plan retry was caused by a manifest missing an explicit race command. The manifest now runs actual -race tests; no gate bypass occurred.

## Production outcome

- API and Worker are clean builds of e5ec5e8beb5dd533885b252d2757ec1dba02a356; both active. No migration or provider/account credential change.
- Independent saved/running route matrix: 37/38 panels passed across the verification window. Pending: d21eb338-a902-462b-9873-ba467781fca3. The additional panel 7d842cca-5728-4fcb-a298-6dfd228af131 passed all 34 probes at 11:12:08Z using a read-only diagnostic with 15-second request timeouts; production timeouts and DB health were unchanged. This is not a claim that every periodic freshness check succeeds.
- Successful real canary: 024a75e3-893d-4642-81a3-30d4f8641302. Residential DNS A/AAAA/HTTPS65 over UDP and TCP all responded. AAAA answers empty per IPv4 policy; HTTPS65 returned real answers. Ordinary Google HTTP3 and HTTPS passed. Ads HTTPS and HTTP3 failed, consistent with independently reproduced SOCKS AUTH_REJECTED.
- First production canary was automatically rolled back. Its DNS test used reserved 192.0.2.53, correctly blocked by the existing destination-IP guard; later API probes proved blocked for that target and managed DNS for public 9.9.9.9. A cold Direct UDP query also timed out. Original failed logs remain; the successful run changed the test target and records cold start separately.
- Cold controls took 7.17 and 6.13 seconds; warm Residential non-ad HTTPS took 0.084 seconds. These are bounded diagnostic samples, not evidence of a sustained maximum-speed improvement.
- Both profile lifetimes remain zero, targets one per inbound and creation intervals 60 seconds. Gate and profile snapshots compare equal before/after.
- Latest source should continue from this handoff and acceptance JSON. Do not restore old all-UDP blocking or all-traffic Residential policy. Do not bypass authentication, invent provider settings, force health green, or expose secrets.
- Next: correct the current upstream authorization through its actual provider/account settings, then repeat Ads TCP/UDP/HTTP3 on the same endpoint. Investigate slow panel route API/cold tunnel latency and Asho SDK timing separately.
