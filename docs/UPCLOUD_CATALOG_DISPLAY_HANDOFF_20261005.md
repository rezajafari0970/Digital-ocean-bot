# UpCloud catalog prices and image labels — 2026-10-05 UTC

Runtime API/worker and changed static app.js/index.html: 46e928d234b4e0c3ee0698a2823863efe3c57103. Other static assets and migration153 unchanged. Read UPCLOUD_CATALOG_DISPLAY_ACCEPTANCE_20261005.json.

User screenshots at 2026-10-06 01:23 Tehran show successful authenticated preview with Madrid regions, plans and Ubuntu images. This confirms Account/Capacity/Catalog now pass after b44d371. The remaining visible issues were all plans showing Price unavailable and two distinct 24.04 options collapsed to identical labels.

Prices: optional GET /price through existing account HTTP transport under one8s budget. Match exact zone.name and server_plan_+plan.ID; amount1; positive finite numeric/string price. Divide by100 and estimate monthly672h, matching official UpCloud CLI 3313774ed86afbfb80cf19537a5486274c62b8a8 internal/commands/server/plan_list.go. Currency comes from prices.currency, or /account then escaped /account/details/username if absent. Missing/invalid currency never assumesUSD. HTTP/malformed/duplicate-zone/timeout pricing remains unavailable; otherwise valid catalog remains usable. No direct fallback/retry.

UI New/Edit: show correct primary-region currency/hourly/estimatedmonth in all3plan selectors, refresh all on primary region change while preserving valid IDs. Note explains primary-region scope,672h estimate,tax/extras. Full Ubuntu title plus actual template_type and architecture; ID suffix only if labels stillcollide. Keep every distinct imageID; no arbitraryversiondedupe. OS defaults use metadata versions. DO/Vultr scalar prices remain compatible.

Validation: focused price/schema/currency/units/unknown/failure/timeout tests; fullGo; race/isolatedPostgreSQL; mobile/desktop New/Edit browser tests including sameversionvariants, metadata defaults, all3price refresh and selectionpreservation pass. OpenAI planning/orchestrator COMPLETE. Reversible deploy includes API,worker,app.js,index.html with local served-byte/readiness/configuration checks.

No saved UpCloud account at baseline, so new authenticated price response still awaits user's Validate retry. No cloud create/delete performed. Live syntheticinvalidtoken probe in acceptance is only reachability/diagnostic proof, not the user's token result. Preserve proxy healthgate, routes and permanent residential profile. No secrets captured.

Backups/evidence: /root/backups/dob-upcloud-catalog-display-20261005. Rollback restores api.before/worker.before and static.before/app.js,index.html, then restarts API/worker; no DB/config rollback needed.
