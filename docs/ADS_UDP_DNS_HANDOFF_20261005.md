# Ads UDP and managed DNS — 2026-10-05

Status: source and installed-core tests passed; production canary pending.

Latest user authorization supersedes the earlier blanket Residential UDP denial: enable UDP, keep only Ads plus the temporary BrowserLeaks exception on Residential, and keep ordinary traffic and DNS direct from the VPN server.

Changes: matched Ads TCP/UDP use SOCKS, HTTP protected UDP remains domain-scoped blocked, other UDP is direct. Residential plain DNS on UDP/TCP 53 is intercepted into the cached IPv4 Cloudflare/Google TCP pool, independent of residential endpoint health; non-IP queries are forwarded by the DNS outbound through the managed direct outbound. Existing Direct identities bypass the new DNS interceptor. Encrypted DNS is not intercepted. No arbitrary domain expansion or provider credential mutation.

Current upstream observation: only one enabled record remains, ID ca78ed2f-0649-4d79-8430-f4ac11233445, SOCKS5, status down; last success 2026-10-05T10:22:07Z. The endpoint is not the prior ProxyRack hostname. Native upstream HTTPS tests failed with curl35. UDP/HTTP3 on this endpoint is unconfirmed. Never claim a successful ad display or attribute this endpoint's limits to the old provider.

Output already exports xtls-rprx-vision-udp443. No exporter change was necessary. Asho client source is not part of this working tree; SDK timing and Android Private DNS still require app-side validation.

Evidence: /root/backups/dob-ads-udp-dns-20261005. Focused race, installed-core fault tests, full go test ./..., syntax/diff checks passed. The DNS test verifies UDP/TCP A answers, cache hits, failed-primary fallback, empty AAAA under IPv4 policy, and real forwarded HTTPS65 NXDOMAIN. Non-Ad UDP delivery and matched Ads TCP/UDP fail-closed are tested against local sinks.

First UDP fixture reused a socket/destination and its direct positive control failed; the fixture now uses a separate source socket per destination. Original failure log is retained. No pass was claimed from a failing positive control.

AI OS Development Orchestrator/API plan used; initial plan retry was caused by a manifest missing an explicit race command. The manifest now runs actual -race tests; no gate bypass occurred.
