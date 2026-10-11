# Residential domain root extension — 2026-10-11

User authorized the listed domain roots and every subdomain, alongside the existing categories. This includes non-ad payload. All URL paths on these hosts are covered by Xray domain routing.

Roots: googleusercontent.com, gstatic.com, googleapis.com, google.com, facebook.com, fbcdn.net, googlesyndication.com, doubleclick.net, 2mdn.net, dns.google.

The shared expanded-category policy supplies the same scope to public TCP/UDP routes, inner pool routes, relay donor inlets, category digests and future panels. Existing all-panel selector activates this fleet-wide. Explicit DIRECT clients preserve their class. DNS infrastructure to approved IPs on port 53 and the Cloudflare DoH exception remain unchanged; dns.google HTTPS now uses residential routing. Client www.gstatic.com, connectivitycheck.gstatic.com and www.google.com exceptions no longer shadow the authorized residential domain rules. Health probes remain independent transport checks.

Boundaries and fail-closed behavior are retained. No credentials, billing, account policy, lifecycle, proxy pool membership, safety gate, geodata files or database schema change.

Validation: focused routing/acceptance tests, full Go suite and focused Race PASS. Native acceptance must run on one live panel before publication, as requested. The ordinary worker reconciliation and plan verification remain enabled after rollout. Synthetic routing evidence does not establish ad asset download, show-rate recovery, or resolution of earlier relay availability limitations.

Operational receipts: /root/backups/dob-ad-domains-release-20261011.
