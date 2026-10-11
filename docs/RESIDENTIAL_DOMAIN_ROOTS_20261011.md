# Residential domain roots — correction at 2026-10-11 08:16 Tehran

Current explicit roots and all subdomains: googlevideo.com, googleapis.com, fbcdn.net, googlesyndication.com, doubleclick.net, 2mdn.net, googleusercontent.com.

These remain alongside the existing geosite categories and compatibility rules. User explicitly removed the broad gstatic.com, dns.google, facebook.com and google.com additions. dns.google appeared in both requested lists; the later explicit removal takes precedence.

Existing category-based routing remains authoritative for domains no longer in the explicit root extension. Existing exact TCP 80/443 client connectivity exceptions for www.gstatic.com, connectivitycheck.gstatic.com and www.google.com are restored automatically by the shared precedence filter; no blanket DIRECT or blanket BLOCK rule was added for their parent domains. Client dns.google returns to the prior strict allowlist policy.

The same scope is used for public routes, inner pools, donor relay inlets, category digests and future panels. Account proxies, credentials, lifecycle, geodata, safety gates and schema are unchanged.

User explicitly requested no tests. Test definitions and operator expectations were updated for consistency but no unit/race/native acceptance test was executed for this correction. Previous acceptance receipts concern the superseded ten-root policy, not this corrected policy. Only clean build, publication integrity and service status are checked.
