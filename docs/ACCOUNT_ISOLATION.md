# Account Isolation

Each provider account is a strict security boundary.

Rules:
- Never share an HTTP client with mutable session state across accounts.
- Never share cookie jars, browser profiles, session tokens, CSRF tokens, refresh tokens, or login state.
- Every account receives its own network context and assigned proxy policy.
- A job carries exactly one account_id and may only acquire that account's context.
- Secrets are referenced by account-scoped secret references; they are never copied between accounts.
- Proxy failure on proxy_required accounts fails closed; there is no direct fallback.
- Logs must never contain cookies, authorization headers, passwords, API tokens, or proxy passwords.
- Persistent session data, if ever required by an integration, uses an account-specific encrypted namespace.
- Reusing a session from another account is treated as a hard error, not a retry condition.

DigitalOcean API access should use account-specific API credentials rather than browser cookies.
