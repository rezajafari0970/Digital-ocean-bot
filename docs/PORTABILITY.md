# Portability

The project must be reproducible on a clean Linux server.

Rules:
- Runtime state is never stored only in the Git working tree.
- Secrets are never committed.
- Schema changes are versioned under migrations/.
- Host-specific paths and ports come from environment/configuration.
- Bootstrap and deployment automation live under deploy/.
- Provider, proxy, provisioning, and panel integrations remain modular.
- Production operations must be idempotent and resumable.
- A proxy-required account must fail closed; it must never silently fall back to the host IP.
