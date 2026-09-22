# Per-Account Security Profiles

Every account receives separate instances and state for network, TLS, HTTP, and browser integrations.

Required isolation:
- TLS config instance and client session cache are never shared across accounts.
- HTTP client, cookie jar, transport, and connection pool are never shared across accounts.
- Browser integrations use account-specific profile directories and storage/cache/auth namespaces.
- Network/proxy policy remains account-scoped and proxy-required routes fail closed.
- Security policy may be common (for example a safe TLS minimum) while runtime state remains separate.
- Profiles are versioned so jobs can record the version they started with.
- Cross-account reuse is a hard error and must be covered by automated tests.

These controls provide isolation and leak resistance. They do not rely on deceptive browser or TLS fingerprint spoofing.
