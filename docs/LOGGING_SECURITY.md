# Logging Security

Logs and error messages are treated as a possible exfiltration path.

Controls:
- Authorization, Cookie, Set-Cookie, Proxy-Authorization, and API-key style headers are always redacted.
- Bearer credentials embedded in ordinary error text are redacted before logging.
- Provider tokens, proxy passwords, browser sessions, and decrypted secret values must never be passed directly to loggers.
- Secret-store errors are intentionally generic for decryption failures.
- Master keys are loaded from a protected runtime file by preference; a base64 environment variable is supported for controlled development environments.
- Runtime master-key files are outside Git and should use restrictive filesystem permissions.
