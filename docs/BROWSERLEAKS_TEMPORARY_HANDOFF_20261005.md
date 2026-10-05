# Temporary BrowserLeaks test — 2026-10-05

User explicitly requested BrowserLeaks as a temporary Residential test exception.
Product commit f8b18ef1c3da62b71889d799abed39f7dced10c6, deployed from a clean detached worktree. Worker and web static only; API binary unchanged.
The exception is domain:browserleaks.com (apex + subdomains), TCP only.
It is stored separately from adDomains in residentialTestDomains. No automatic expiry was requested.
Ads TCP-only, Direct TCP/UDP isolation, Residential UDP blocking, and direct DNS/other TCP remain unchanged.
Matched protected domains fail closed if the Residential upstream fails. Lookalike domains are not included.

Focused/race/full/clean-build tests, installed-Xray packet faults, mobile UI link and real Reality connection passed.
Real BrowserLeaks HTTP200 displayed a non-server Residential IP; non-Ad and opaque-IP probes still used server IP.
Canary passed 34 checks. Full matrix passed on 35 panels; separate BrowserLeaks Direct/Residential probes passed
on another 2 panels. Total BrowserLeaks runtime confirmation: 37 of 38.
Panel 49ff99be-045e-4eb9-b3e9-a5d2248b99ee remains unconfirmed because its route API times out/returns success=false.
Saved template has the exception; do NOT infer that runtime succeeded on this panel.
Periodic routing proofs also have API timeouts; some existed before this change.
A slow-panel probe took 2.8–9 seconds per request; do not repeatedly restart merely for proof deadlines.

Both services active. Profiles, mutation gate and upstream credentials were not edited.
Evidence: /root/backups/dob-browserleaks-test-20261005
Current verification snapshot: 2026-10-05T09:26:18.491714+00:00. Read BROWSERLEAKS_TEMPORARY_ACCEPTANCE_20261005.json for exact counts.
The next investigation should separate route API latency from actual Xray dataplane health.
Keep it scoped and read-only first. Do not restore Residential DNS/opaque catch-all or UDP.
