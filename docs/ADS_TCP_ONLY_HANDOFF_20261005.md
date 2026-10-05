# Ads TCP-only continuation — 2026-10-05

Production verified at 2026-10-05T08:43:51.570970+00:00. Product commit: 8d95254d0ce69ea35b5e2de39f36dca9323bb551.
Canonical branch: checkpoint/final-e2e-20260929. Server and working tree remain authoritative.

The latest explicit user instruction is ONLY Ads categories through Residential and no UDP.
This supersedes the previous DNS/opaque-IP/BrowserLeaks protective routing and UDP rollout.
Residential now proxies only TCP matching category-ads-all, category-ads, google@ads, facebook@ads.
Residential UDP, including DNS/QUIC, is denied. Other TCP uses server egress.
Direct clients retain TCP/UDP routing. Matched Ads have no direct fallback when the proxy fails.
Missing/invalid sniffing still blocks residential clients. Unobserved inbounds remain fail-closed.
DNS pool remains Cloudflare, Google and AliDNS TCP. No proxy credentials or policy profiles changed.

Focused, race, full, clean detached full, real installed Xray fault/packet tests, UI and Output class checks passed.
Canary passed 26 route checks; 37 unique production panels passed 962 route checks across the acceptance window.
First fleet read had 36 passes/1 pending while plans changed; fresh isolated recheck passed.
Final durable state: 37/37 APPLIED with proof age under 60 seconds.
Real Reality connection: non-Ad and opaque-IP traffic exited with server IP; Google204, BrowserLeaks200,
and Ads transport404 succeeded. Ads404 is not an ad-playback or egress-IP attestation.
No 30,000-concurrency or upstream capacity certification is claimed.

Worker and web static were deployed from a clean detached build. API binary remains at its prior revision.
Execution gate and config-generation profiles were preserved. No provider/account deletion was attempted.
Evidence and scripts: /root/backups/dob-ads-tcp-only-20261005
Read docs/ADS_TCP_ONLY_ACCEPTANCE_20261005.json for exact assertions.
The acceptance command defaults read-only; UUID followed by --apply requires an inactive worker and one eligible panel.
Never run its mutation mode concurrently with Worker.

Pending unrelated user request: explain and approve the config-generation/Output interval strategy before altering it.
Do not restore residential DNS/default catch-all or UDP based on obsolete handoffs.
