# Lifecycle State-Machine Brain

Commit `03f810813368f1902cb85f42d8de0d9d8d48c198`. Indexed **60 lifecycle/provisioning/worker files**, **54 state-like constants**, **69 transition evidence locations**, **3 timing/retry locations**.

This is static evidence, not a formally proven state machine. Before changing a transition, inspect its exact source, owner and persistence behavior.

## State-like values
- `ALLOW_CREATE` — 1 references
- `BOOTSTRAPPING` — 1 references
- `COMPLETED` — 2 references
- `CONFIGURING_PANEL` — 4 references
- `CREATE_DROPLET` — 6 references
- `CREATE_TERMINAL_FREEZE_V2` — 1 references
- `CREATING` — 5 references
- `DATABASE_COMPLETE` — 4 references
- `DELETED` — 10 references
- `DELETE_DROPLET` — 4 references
- `DNS_RESOLUTION_FAILED` — 3 references
- `FAILED` — 18 references
- `INSTALLER_NOT_CONFIGURED` — 2 references
- `INSTALLING` — 5 references
- `INSTALLING_PANEL` — 1 references
- `INSTALL_COMPLETE` — 9 references
- `INSTALL_FAILED` — 4 references
- `INSTALL_ROLLED_BACK` — 4 references
- `PACKAGE_HEALTH_FAILED` — 1 references
- `PACKAGE_STATE_INCOMPLETE` — 3 references
- `PANEL_COMPLETE` — 4 references
- `PENDING` — 1 references
- `PHASE_FAILED` — 3 references
- `PKG_DEPENDENCY_FAILED` — 2 references
- `PROBE_FAILED` — 2 references
- `PROVISIONING` — 7 references
- `PROVISION_TERMINAL_FREEZE_V1` — 1 references
- `READY` — 21 references
- `REBOOT_REQUIRED` — 1 references
- `RESOURCE_DELETED` — 2 references
- `RETRY` — 5 references
- `RETRY_WAIT` — 1 references
- `RUNNING` — 1 references
- `RUNNING_SCRIPT` — 1 references
- `SERVICE_START_FAILED` — 3 references
- `SSH_AUTH_FAILED` — 2 references
- `SSH_COMMAND_EXIT` — 4 references
- `SSH_COMMAND_FAILED` — 5 references
- `SSH_CONNECTION_REFUSED` — 4 references
- `SSH_CONNECTION_RESET` — 3 references
- `SSH_CONNECT_TIMEOUT` — 3 references
- `SSH_CONTEXT_TIMEOUT` — 3 references
- `SSH_DISCONNECTED` — 3 references
- `SSH_HANDSHAKE_FAILED` — 3 references
- `SSH_HOST_KEY_FAILED` — 2 references
- `SSH_HOST_KEY_MISMATCH` — 1 references
- `SSH_HOST_KEY_VERIFIER_MISSING` — 1 references
- `SSH_NOT_READY` — 3 references
- `SSH_NO_ROUTE` — 4 references
- `SSH_UNKNOWN` — 1 references
- `STAGE_FAILED` — 1 references
- `WAITING_INSTALLER` — 10 references
- `WAITING_RESOURCE` — 4 references
- `WAITING_SSH` — 1 references

## Ownership areas
- Compute lifecycle/reconciliation: `internal/droplets`
- Provisioning/install execution: `internal/provisioning`
- Worker ownership: `internal/worker`, `cmd/worker`
- Scheduling: `internal/scheduler`, `internal/app/scheduler.go`
- Deployment/recovery orchestration: `internal/app`
