# Transport experiment removal — 2026-10-06

The user canceled transport experimentation and requested removal of everything introduced from the original FinalMask request onward. This supersedes the previous activation and P3/P4/P6 plans. Do not re-enable or continue those experiments.

## Removed

- Output Client FinalMask controls, admin GET/PUT endpoints, per-panel URI overlays and the added Xray JSON export extension.
- Internal profile implementation, feature tests, research, activation evidence and development job.
- All stored profile/operation rows and both feature tables, via migration156. Migration155 remains byte-for-byte immutable upgrade history; it is immediately retired by156 on new installations. The removal down migration intentionally does not restore retired data.
- The uncommitted P3/P4/P6 pilot worktree, local feature branches, downloaded cores, Samizdat source/test binary, private test configs, credentials in test fixtures, browser profiles, source inspection files and the three feature/pilot backup directories.

## Production and preservation

API and static assets restored from the verified clean pre-FinalMask release, source `d384c8b054f57c4b24cbea57bc58e05c5317fd04`. All application code/static files match that source exactly. Worker digest is unchanged. Migration156 applied successfully. Existing UpCloud, residential settings/publication, proxy/account configuration and Reality profiles compare equal before/after removal.

Current Direct and Residential exports each contained30 links and zero `fm` parameters during acceptance; the shared Direct subscription also contained30 unmodified Reality links. GET of the retired endpoint returns404; PUT returns the generic405. The former JSON-format query now has baseline plain-URI behavior. Readiness and existing account/residential/output APIs passed.

## Fleet

Thirty-four non-deleted managed panel instances were enumerated, including one expiring server. Thirty-three were authenticated via Sanaei API and pinned SSH: no FinalMask keys or pilot protocols in saved/runtime configuration; no experiment files, units or processes in the inspected deployment locations; x-ui active. Target209.222.30.208 retains the exact pre-experiment runtime file SHA256 `63b69802a43173833e5e4c9e5baaf95283aaba53f0960a132ed8568f141af5d4`.

95.179.245.113 did not answer direct API/SSH checks. A bounded peer attempt through209.222.30.208 reached TCP22/2053 but did not complete authenticated SSH inspection. The host remains UNVERIFIED; no changes were made on it. This access failure is not proof of any experiment residue. No experimental service had been installed on fleet servers by the canceled work; FinalMask was applied only to client exports.

The preexisting `/usr/local/libexec/config-manager-sing-box` binary dates to2026-09-17 and belongs to earlier functionality; it was deliberately preserved. Historical Git commits and audit events remain history, not active features. Imported phone configs are external copies: users must refresh subscriptions/reimport the current clean links and reconnect.

## Verification

Migration integration covers absent feature tables, populated migration155 tables, and repeated removal. Focused admin API tests with the race detector passed. Static syntax, pre-feature source parity, service readiness, retired routes, current/shared output and preservation snapshots passed. AI OS PLAN response and redacted per-server results are recorded in `TRANSPORT_REMOVAL_ACCEPTANCE_20261006.json`.
