# UpCloud server-list query correction — 2026-10-05 UTC

Runtime API/worker: b44d3716e104bbc916caf311d29189940151c36f, clean detached build. Static remains cb131196167337612de85fc44d3005ffa9eae7ee; migration remains 153. Read UPCLOUD_SERVER_QUERY_ACCEPTANCE_20261005.json.

The user trace 84ebfb01b8b8f3db at 21:41:39 UTC (2899ms) passed Account parsing after 1ae9121, then failed capacity/list_servers with HTTP400. Current official OpenAPI https://developers.upcloud.com/api/1.3/upcloud-openapi.json defines sort_by as the field (title etc.) and order_by as the direction (asc/desc). The legacy https://developers.upcloud.com/1.3/8-servers/ prose reverses these names. Source sent the legacy inverted query.

Product change: /server?limit=100&offset=N&sort_by=title&order_by=asc. Pagination, complete-inventory safeguards, bounded requests and error classification remain unchanged. No retries, direct fallback, swallowed errors, credential handling, proxy selection, DO/Vultr, quota parsing, routes/gates/schema/static or permanent residential profile changes.

Contract fixtures reject inverted requests with HTTP400 and failed before the patch. They now pass for empty inventory, short pages continued to empty, 201-server full-page inventory, first/later-page400 with no partial results and no retry. Existing inventory/adoption/capacity tests, full Go, race with isolated PostgreSQL, and browser gates pass. OpenAI planning/orchestrator COMPLETE.

Production API/worker are healthy and before/after configuration matches. Read-only local API checks pass. The synthetic invalid-token probe outcome is recorded in acceptance JSON; it cannot prove authenticated server listing, and is not a rejection of the user's credential. There were zero saved UpCloud accounts at baseline. No live UpCloud create/delete performed.

Next: user refreshes panel and clicks Validate with their existing token. Confirm complete authenticated preview with a new trace, and address any later-stage evidence independently. Do not request/capture credentials or raw provider bodies. Full preview success is still unconfirmed.

Evidence/backups: /root/backups/dob-upcloud-server-query-20261005. Rollback restores api.before/worker.before (1ae912189edcac45437b1ede21f17743cd3f6942) then restarts the API and worker. No schema/static/config rollback needed. Canonical current-frontier docs supersede older UpCloud handoffs for runtime state.
