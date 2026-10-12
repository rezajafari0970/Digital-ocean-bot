# Account Direct and Residential Output

Every Accounts card offers Direct and Residential buttons. Each opens the existing live, plaintext viewer for the selected account's active eligible configs. It refreshes every second, clears on fetch failure/hidden page, and retains the same capability. URI strings are not rewritten.

## Authority and compatibility

POST /api/v1/accounts/{id}/output/share uses the same authenticated write permission as existing global shares (Admin and Operator; Viewer rejected). JSON route_class is required and must be DIRECT or RESIDENTIAL. Malformed requests are 400, absent/soft-deleted accounts 404, storage failures 500.

Capabilities live in the separate account_output_share_tokens table, with a required account UUID, class CHECK, and ON DELETE CASCADE. GET /share/account-output/{token} reads that fixed scope. Account/class URL overrides are rejected. Existing output_share_tokens and /share/output remain global and unchanged. A scoped token is never inserted into the legacy table and cannot be used on the legacy path, including with an old binary.

The shared selection helper binds the account and verifies panel, droplet and deployment account agreement. The active token is rechecked in the same SQL statement selecting output. Account soft deletion and token revocation committed before that statement prevent publication; already-running statement snapshots/bytes cannot be recalled. A nil scope is reserved for existing global calls; invalid nonnil scope is rejected.

Existing Output freshness, route proof, immutable profile ownership, profile ports, server protection, cleanup, residential-health, expiry and narrowing time windows remain authoritative. Empty/unavailable accounts never fall back to global output. This feature does not create configs, change network routing, provider settings, proxy assignment, account quotas or server lifecycle.

## Review findings and evidence

The first design council requested revision. The mixed-version/rollback finding was resolved by using a separate capability table and endpoint instead of a nullable column in the global table. Token revalidation is in the final query. Scan errors fail the complete response. The existing Output coordinator prevents concurrent polls for the same panel; focused coordinator race tests are retained. No supported production API transfers a panel/droplet/deployment to another account. Privileged manual database ownership changes are outside this interface.

Tests use an isolated bulk_test database, never production: four providers, identical account names, copied client identity without local ownership, concurrent fixed-token reads, multiple panels, both classes, exact URI preservation, global compatibility, tampering, invalid requests, stale snapshots/proof/health, expiry, contradictory owners, deletion, revocation between lookup and selection, closed database and actual authenticated routes. Browser fixture covers eight buttons, simultaneous popups, raw output, fixed-token expiry refresh and mobile/desktop layout.

Initial fixture attempts are retained: duplicate owned client IDs were rejected by the existing global uniqueness constraint; the corrected fixture tests copied unowned identities instead. Early browser fixture required explicit automation user activation and popup handling. These are not production failures.

Baseline and rollback source: d73e7205f54bc072e2446c59b5949ce2adbc5af5. Migration 170 is additive. Older code cannot consume these capabilities even if the table is retained. Downgrade may remove the new table after newer readers stop; do not alter legacy tokens. Release must use a clean source checkout and existing locked deployment/rollback procedures. Historical canonical detached-branch bootstrap assertion fails; exact source/runtime provenance is verified independently without claiming mastery acceptance.

Evidence directory: /root/backups/dob-account-output-20261012.
Release and measured API totals are recorded there in completion.json and progress.json.
