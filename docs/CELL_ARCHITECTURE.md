# Account Cell Architecture

Each cloud account is an independent cell managed by the orchestrator.

A cell owns its tenant context, credentials reference, network profile, proxy route, transport/session state, cache namespace, rate state, circuit-breaker state, jobs, operations, resources, and audit scope.

Core invariants:
- A worker must acquire the target account cell before executing a job.
- A job or operation belongs to exactly one account.
- Provider mutations use an idempotency key and a persisted operation ledger.
- Unknown outcomes are reconciled before a mutation is retried.
- Runtime failures affect only the owning account cell.
- Cross-account state access is a hard error.
- Proxy-required cells never silently fall back to a direct route.
- All durable workflow checkpoints live in PostgreSQL so a process or host restart can resume safely.

The same invariants and migrations are part of the repository so deployments on future servers reproduce the architecture.
