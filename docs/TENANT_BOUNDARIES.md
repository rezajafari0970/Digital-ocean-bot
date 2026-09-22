# Tenant Boundaries

Every cloud account is an independent tenant.

Isolation requirements:
- Dedicated provider credentials and secret namespace.
- Dedicated proxy/network profile.
- Dedicated HTTP client, cookie jar, transport, and connection pool.
- Dedicated session/auth state where an integration requires it.
- Dedicated cache, rate-limit, job, and audit namespaces.
- No cross-account fallback or borrowing of state.
- Jobs must carry one immutable account ID.
- Cross-account context use is a hard error.
- Provider integrations should prefer official APIs and stable truthful protocol headers; isolation must not depend on fingerprint spoofing.

Portability requirement:
These boundaries are code/config/database invariants and must reproduce on every deployment host.
