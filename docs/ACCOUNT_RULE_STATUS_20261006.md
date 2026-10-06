# Account rule status correction — 2026-10-06

Live inspection at 13:41 UTC found all four visible accounts enabled for apply-to-existing, revision 0, no replacement revision, no shrink intent and no waiting resource. Enabling alone did not queue a rollout. Sara vul 2 retained Desired 15, Lifetime 3540–3720 seconds, Build spacing 1–3 minutes.

The frontend previously combined ON with the persisted initial FUTURE_ONLY label and showed a generic queued-work Save alert. Display now derives work from replacement_revision, shrink_pending and waiting_droplet_id. ON with no work shows "No replacement queued" and explains that enabling alone does not replace existing servers. Stale next-action time is hidden without work. OFF with admitted deletion or residual work remains visible. Save forces fresh account readback; missing/failed readback says status unavailable. A successful PUT followed by a refresh error is no longer labeled Save failed.

No SQL, backend/worker, build rules, deletion intent, account settings, provider guards or actual server configuration changed. A missing control row remains the established default-OFF behavior and keeps the legacy card compact; it is not inferred as active.

Validation: focused Node state/render/Save tests; existing isolated PostgreSQL and race acceptance across DigitalOcean, Vultr and UpCloud; real headless mobile/desktop account Edit/Save/reload. Existing acceptance passed before the follow-up error-message/OFF residual-state correction; final Node tests cover those changes. No real cloud creates/deletes were used for tests.

Server API source review identified the post-commit error-label issue and OFF residual-state visibility; both corrected. Its missing-card finding was not adopted: the API intentionally represents a never-edited default-OFF account with an empty control object.

Separate observation: Sara vul 2 had 12 READY, 1 EXPIRING and 2 RETIRING managed records, including retirements older than this feature. Worker logged "account network not ready" for one retirement at 13:36 UTC. Provider ACTIVE and Desired satisfied do not prove lifecycle health; no global health/rotation success is claimed.

Deployment: static app.js only, atomic replacement with pre-change backup in /root/backups/dob-account-rule-status-20261006. API/worker binaries remain bd592fb, schema remains 158; no service restart required. Rollback restores the backed-up app.js. Verify served asset SHA against source and service readiness after publication.
