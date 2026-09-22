# Privacy and Leak Hardening

Outbound provider traffic is hardened at the transport boundary.

Security invariants:
- Proxy-required accounts fail closed.
- Internal forwarding/debug headers are stripped before network transmission.
- The original request object is not mutated while hardening a request.
- Leak checks compare the observed public address with the account's expected proxy exit address.
- Seeing the management server public IP is a hard failure.
- Cross-account cookies, sessions, credentials, transports, and connection pools remain prohibited.
- Authorization data and proxy secrets must never be written to application logs.
- Browser/TLS fingerprint spoofing is not a security dependency; official provider APIs are preferred.

Leak tests are part of the codebase so the same guarantees can be verified after deployment to a new management server.
