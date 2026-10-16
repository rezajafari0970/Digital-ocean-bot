# Fleet repair acceptance — 2026-10-16

Recorded: 2026-10-16T11:55:19.252923+00:00. API usage: **0 requests / 0 tokens / USD 0**. Xray core was not modified.

## Installed versions and validation

- Public release: 5dacef92ffeea214cfc6e977bb4341f0b3d40cc4; product source: 11b311b66de595a4b103ed9d41b52935f4fd3281.
- Both service hosts: all three services active, executing hashes match, no automatic restarts, five consecutive readiness observations passed.
- Main/stable CI: 38894224197 / 38894224584, both successful.
- Accounts, environment, creation/output/protection policy preserved. All 221 existing primary listener pins retained unchanged.
- Test panel 21727ecc-3871-4c7b-9653-f1db84b81d44 at 104.105.133.93 remains READY and enabled.
- Protection desired policy remains **OFF, revision 50**. This task did not disable it.

## Completed stages

| Stage | Result and evidence |
| --- | --- |
| Video review | Full 84.474-second sequence reviewed at one-second intervals, key frames enlarged, silent audio confirmed. Detailed ledger: FLEET_VIDEO_REVIEW_20261016.md. |
| Provider/proxy identity scheduling | Independent bounded lanes, rolling discovery and deadlines after admission. The 180-second freshness guard was preserved. Final samples had no provider errors and no SQL control-pool waits. Real upstream failures during the session were retained as evidence; recovery was observed. |
| Fleet enrollment and output coverage | Legacy global cap 64 migrated to all eligible servers. Custom scopes, paused policies, ownership, budgets and serial client execution remain enforced. Actual eligible/enrolled/awaiting counts are displayed. |
| New-panel scheduling | Capacity fill uses rolling two-slot discovery; unattempted panels no longer wait for an all-fleet batch. Operation deadlines begin after admission. Startup convergence and the configured 60-second profile cadence still take time. |
| SSH admission and memory | 92 unique earlier nodes and 10 new nodes, zero overlap, **102 distinct verified nodes** over the changing fleet. Default-only SSH changes retain operator settings and rollback until a fresh pinned connection confirms them. Bounded swap setup now covers 1 GiB hosts. |
| TCP pressure sampling | Kernel TCP memory counters and watermarks added; pressure/recovery and unknown-sample behavior tested. Kernel limits and existing connections are unchanged. Live ON acceptance is not claimed. |
| OFF cleanup and SCP flow control | OFF cleanup no longer waits for an unnecessary guardian upgrade. Ownership, unit hash, supported protocol, revision fence and fresh DISABLED receipt are required. SCP concurrently drains stderr with a 1,024-byte diagnostic bound. |
| Export preferences | Prior 11-surface acceptance verified omission of client Extra, FinalMask and Browser Dialer while preserving saved settings. Export code/policy remained unchanged in the host-pressure and transfer follow-ups. Current output checks found no invalid links. |
| Final profile completion | Two newly ready panels completed DIRECT=1 / RESIDENTIAL=3 through bounded invocations of the standard reconciler. The original 60-second profile interval, executor gate and locks were retained. This operational catch-up is recorded separately from automatic convergence. |
| Publication and installation | Normal publisher, stable update and preservation checks completed on both hosts. Disk-reserve refusals occurred before stopping services. Only completed task fixtures and reproducible caches were reclaimed; production backups were retained. |

## Exact observed dashboard

Snapshot: **2026-10-16T11:50:55.885079+00:00**. Fleet creation and retirement continue under the existing policy, so the total is not a fixed historical count.

| Counter | Value |
| --- | ---: |
| Total tracked | 92 |
| Active | 90 |
| Inactive / unverified | 1 |
| Provisioning (included in inactive) | 1 |
| Degraded / admission blocked | 1 |
| Panels needing attention | 1 |
| Provider check overdue | 0 |
| Broken | 0 |
| Pending deletion | 0 |
| Protection need attention | 0 |
| Protection cleanup pending | 0 |
| Protection paused / reports overdue | 0 / 0 |

Final output window: 2026-10-16T11:50:53.687000+00:00 through 2026-10-16T11:51:24.640497+00:00.
- Direct: 89–90 links; Residential: 267–270 links.
- Final eligible/enrolled/awaiting: 91 / 91 / 0.
- Two observations had complete DIRECT1 / RESIDENTIAL3 coverage of the stable healthy cohort. The last observation withheld one changing route despite healthy boundary snapshots; it is not represented as continuous full coverage.
- All exported links in these samples passed validation. No duplicate client creation, forced health flags or freshness relaxation was used.
- All 93 then-eligible nodes had enrolled earlier; a subsequent lifecycle change reduced the eligible set to 91. The two final catch-up panels have the complete intended ownership counts.

## Remaining limits and unresolved acceptance

1. Real CPU/memory pressure remains on some serving nodes. Pinned inspection of 158.247.225.38, 64.176.228.197, 70.34.246.148 and 45.77.168.104 found Xray RSS around 422–986 MiB and memory PSI full avg60 around 5–11%. Routing verification can still fail or be briefly unverified. Existing swap does not create CPU or physical RAM. No unsafe threshold relaxation or blind restart was used. Removing this capacity constraint requires workload/capacity decisions.
2. Protection ON under real load was not tested because the desired policy is OFF. Zero paused/overdue counts under OFF do not establish enabled-protection acceptance. Large enabled upgrades over pressured links can still exceed the controller transfer budget; the OFF cleanup repair does not claim to solve that separate enabled-upgrade scenario.
3. Actual upstream proxy/provider interruption occurred during the session, including a Vultr identity failure near 11:47 UTC. Fresh identity health recovered at 11:49:00.759 UTC. Permanent upstream availability is not guaranteed or fabricated.
4. Root storage headroom is limited; the last preservation sample showed 5557 MiB free. Deployment reserve guards were retained. No production backup was deleted to make deployment pass.
5. The canonical checkout remains detached under the existing continuity workflow; the separate mastery/continuity gate is not declared READY.

## Tests and durable evidence

- Combined host/TCP/capacity release: full serial suite on an isolated authenticated PostgreSQL instance passed, including the forty-connection proof repeated three times. Focused race tests and vet passed.
- Transfer follow-up: both new regressions failed before correction; provisioning/protection race suites, focused large-payload/cancellation tests, full offline suite and vet passed. This offline suite is not claimed as a fresh database-integrated run; the follow-up changed no schema or SQL.
- A live 4,477,088-byte transfer completed with matching SHA-256 in 112.95 seconds; socket inspection confirmed TCP receive-window pressure. The stderr deadlock was separately reproduced with a controlled SSH peer.
- Detailed private evidence is under the worktree's .local/evidence, including terminal-ui.json, terminal-outputs.json, final-pressure-audit.json, final-target-ownership.json, transfer-actions.json and both deployment verification records.
- Previous failures, safe deployment refusals, retries and transient health changes were retained rather than overwritten.

This report records installed fixes and measured outcomes. It does not claim that every server will remain active forever or that all capacity-dependent behavior has been eliminated.
