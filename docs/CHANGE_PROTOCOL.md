# Change Protocol

## Before touching a feature
1. Locate it in `FEATURE_FLOW_INDEX.md`.
2. Read the UI caller, registered API route and handler.
3. Trace orchestration into provider/panel/network/store code.
4. Read related tests and migrations.
5. Verify current production state/logs if the issue is runtime-visible.

## During implementation
Keep provider-specific behavior behind provider boundaries. Do not bypass account/network isolation. Do not replace Sanaei API live reads with SSH. Do not introduce stale Output behavior. Do not store secrets in Git/docs/logs. Prefer a complete vertical slice over unrelated refactors.

## Verification
Run focused package tests first, then broader tests/build appropriate to the change. For schema changes, verify migration ordering and live migration level. For web/API changes, exercise the real endpoint/UI flow. For deploys, verify API and worker service health plus the feature smoke test.

## Commit/checkpoint
A logical stage is not complete until code/tests are committed, pushed to the canonical checkpoint branch, and `PROJECT_STATE.md` + `HANDOFF.md` reflect the exact deployed/tested boundary. Audit artifacts are preserved unless deliberately reviewed and archived.
