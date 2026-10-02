# Deployment & Server Lifecycle Control Plane — Zero-to-100 Audit

Date: 2026-10-02

## Scope
Reservation/create; ambiguous recovery; SSH identity/readiness; provisioning; installer activation; database/panel continuation; READY finalization; expiry/replacement; delete/reconcile; worker crash/backoff; concurrency/isolation; observability and production convergence.

## State semantics
PANEL_COMPLETE is intentionally terminal for the post-install-only workflow and must not be counted as a stuck/nonterminal deployment. READY, FAILED, INSTALL_FAILED, INSTALL_ROLLED_BACK and PANEL_COMPLETE are terminal states in the current workflow engine.

## Confirmed strengths
- Durable deployment reservation before provider create.
- Provider create operation ledger and identity-tag adoption for ambiguous outcomes.
- Per-deployment SSH identity and secret references.
- Run lease plus account/provider mutation guards.
- Provision step attempt budgets, diagnostics and durable readiness snapshots.
- Installer manifest pinning/generation and rollback support.
- Post-install database/panel continuation is generation-aware.
- Lifecycle uses account-scoped locks, desired-capacity admission and replacement ownership.
- Delete is idempotent and reconciliation is provider-aware.
- Worker FailureStore supplies bounded exponential backoff.

## P0 findings and corrections
1. deployments.lock_version was incremented but not compared. SQLStore is now compare-and-swap and Deployment carries LockVersion.
2. Direct deployment writers bypassed version invalidation. All direct UPDATE deployments paths now increment lock_version, forcing concurrent workflow runners to re-read rather than overwrite.
3. Permanent provider permission/auth errors before the workflow engine could leave PLANNED reservations retrying indefinitely and consuming max_concurrent. Recovery now freezes permanent create preparation/mutation errors to FAILED/done.
4. WAITING_INSTALLER activation used worker-level retry forever for retryable readiness failures. Recovery now has a 64-failure / 30-minute activation budget and terminalizes exhausted installer recovery with diagnostic evidence.
5. Provider mutation errors were not uniformly reflected in account provider state. Create mutation callbacks now propagate account-level auth/permission/locked/rate/transport classes while capacity remains capacity evidence, not account transport state.
6. The initial audit counted PANEL_COMPLETE as nonterminal; this was a measurement error, not a runtime defect.

## Live debt observed before hardening
- 669 deployments total.
- 238 PANEL_COMPLETE historical terminal deployments.
- 1 PLANNED create reservation ~20h old with >1000 worker failures: provider permission_denied.
- 2 WAITING_INSTALLER deployments, one with >1200 SSH_CONNECTION_REFUSED failures and one with >100 installer-incompatible/reboot-required failures.
- 9 expired live droplets: 5 RETIRING on a LOCKED DigitalOcean account with failure/backoff; 4 EXPIRING on an ACTIVE DigitalOcean account.
- The four active-account EXPIRING droplets were blocked behind the stale PLANNED reservation consuming max_concurrent=1.

## Verification evidence
- go test -race workflow/provisioning/droplets/app/testharness PASS.
- full go test ./... PASS after each checkpoint.
- deployment CAS validated against production schema inside BEGIN/ROLLBACK: fresh CAS updates one row and stale CAS updates zero.
- production canary showed both historical WAITING_INSTALLER deployments converged to INSTALL_FAILED after the new recovery budget.
- historical permission-denied PLANNED reservation converged to FAILED after permanent-error recovery was applied, releasing the lifecycle concurrency slot.
- lifecycle subsequently created a replacement reservation for the oldest EXPIRING resource, proving the previous replacement deadlock was removed.

## Remaining external-state behavior
Resources on provider accounts that are LOCKED/PERMISSION_DENIED are intentionally fail-closed. The control plane must preserve local history and backoff rather than blindly create/delete until provider authority recovers.

## Production completion evidence
Production product commit 9846aceeea667b876b0e93563a1127e206ee2fbc is verified: runtime commit, manifest commit, API/worker hashes and active services all match.

After promotion:
- both historical WAITING_INSTALLER loops are terminalized; no WAITING_INSTALLER remains.
- the stale permission-denied PLANNED reservation is terminalized; no historical PLANNED reservation remains.
- the previously ACTIVE DigitalOcean account that rejected create/SSH-key mutations now correctly reports provider_state=PERMISSION_DENIED and runtime_status=PROVIDER_PERMISSION_DENIED.
- its four expired EXPIRING resources remain preserved fail-closed with no replacement churn while provider mutation authority is denied.
- five RETIRING resources on a separate LOCKED DigitalOcean account remain preserved with lifecycle failure/backoff.
- lifecycle demonstrated recovery after the stale reservation was removed by creating a replacement reservation; after the provider permission error was observed, account-level blocking stopped further churn.
