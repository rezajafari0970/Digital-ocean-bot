# Metadata Isolation

Internal account topology must not leak into outbound provider requests.

Controls:
- Runtime cell, request, correlation, and audit scopes use independent cryptographically random identifiers.
- Internal account, cell, and job identifiers are removed at the outbound transport boundary.
- Outbound request IDs are generated independently and are not derived from account IDs.
- Mutable network/session/cache/rate/circuit state remains account-scoped.
- Logs and traces may correlate activity internally, but secret or account topology metadata must not be sent to providers unless required by their documented API.
- Standard protocol behavior and provider-required headers remain truthful and interoperable; this layer is for privacy and isolation, not deceptive fingerprint spoofing.
