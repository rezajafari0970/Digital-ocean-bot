# UpCloud nullable quota fix — 2026-10-05 UTC

Runtime API/worker: 5aa7fef66b5ff3e644663b8468d4a724585aa6dd, clean detached build. Static UI remains cb131196167337612de85fc44d3005ffa9eae7ee and migration remains 153. Read UPCLOUD_NULL_QUOTAS_ACCEPTANCE_20261005.json.

User screenshot trace ab50daf03fb50a00 correlates with production journal at 20:53:49 UTC: stage account, operation account, provider HTTP 200, INVALID_NUMBER, 2163 ms. This supersedes the earlier unknown-stage diagnosis. The response body and submitted token were not retained. The precise original field/value cannot be recovered.

The current official UpCloud OpenAPI accountResourceLimits explicitly allows integer|null for cloud_server_dev_1xcpu_1gb_10gb_plans and cloud_server_dev_1xcpu_1gb_plans. Our old map[string]number decoder rejected both. A synthetic protocol fixture reproduced the same account failure before the patch; Account -> Capacity -> Catalog now passes. This is a demonstrated schema bug consistent with the user failure, not a claim to have inspected their raw response.

Changes:
- Dedicated resourceLimits decoder accepts null only for those two keys and retains nil distinctly from numeric zero.
- Ordinary plans ignore unrelated nullable Dev quotas. Selecting a plan whose present quota is null returns UNKNOWN_PLAN_QUOTA and cannot authorize capacity.
- Negative, fractional, overflow, malformed and unexpected null quotas still fail closed. Existing quoted numeric strings continue to work; this intentionally preserves existing protocol compatibility despite the planning model suggesting their rejection.
- Shared number decoder, selected proxy transport, sticky session, DO/Vultr, cleanup, mutation gates, schema, static UI and permanent fleet residential profile unchanged.

Regression before/after, full Go, race with real isolated PostgreSQL, browser/mobile checks and configuration comparison passed. The existing provider-level fixture exercises the same three methods as accountPreview; unchanged staged handler tests also passed. OpenAI-backed orchestrator is COMPLETE.

Production readback passed. The synthetic invalid-token probe is only reachability/staged-error evidence and must never be presented as rejection of the user's credential. There were no saved UpCloud accounts at baseline. No real UpCloud create/delete was performed.

Next: user refreshes and retries Validate with their token in the panel. If it succeeds, complete normal account setup. If it fails, correlate the new stage/reason/ref; do not ask for credentials in chat. If UNKNOWN_PLAN_QUOTA occurs later for a selected Dev plan, do not silently interpret null as unlimited; establish provider semantics or choose a plan with known quota through the normal user flow.

Evidence/backups: /root/backups/dob-upcloud-null-quotas-20261005. Binary-only rollback restores api.before/worker.before (88f8b50) and restarts services; no schema/static/config rollback needed. Never reset gates, publication or user proxy choices as part of this fix.
