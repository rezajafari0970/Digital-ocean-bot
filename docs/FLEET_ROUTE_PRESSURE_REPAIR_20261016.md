# Fleet route and native-resource repair — 2026-10-16

## Confirmed causes and changes

1. Transport apply retry exhaustion used BLOCKED even when every attempt failed before native backup, remote listener identification or mutation. Six live panels had no native inbounds; these pre-write jobs then prevented automatic listener repair and client admission. Exhausted forward jobs now become FAILED_BEFORE_WRITE only with an empty backup reference, absent remote/restart markers, a known pre-write phase, no rollback parent and an unchanged source profile. The update runs on the connection holding the executor advisory lock. Unknown outcomes and rollback jobs remain blocked.
2. On 172.237.8.215, the native profile contained 16,424 goroutines, including more than 10,000 in REALITY fallback paths, with 21,092 pipe descriptors. Source inspection confirmed unbounded TCP splice copying and no fallback idle limit. Native fallback now uses explicit 16 KiB buffers and a shared 60-second idle clock. Active one-way traffic and half-close responses remain supported. Configured rate-limit waits are interruptible on expiry.
3. On 172.236.162.223, 5,581 goroutines were stuck flushing REALITY handshake responses; total goroutines were 19,041. A separate 60-second handshake deadline now interrupts stalled reads/writes. Ownership transfers to authenticated transport or the fallback idle clock; the deadline cannot later close a successfully handed-off connection.

Authenticated transport configuration, listener pins and client credentials are not rewritten by these native corrections. Normal core startup validation, native identity digests, backup journals, fresh running-image verification and rollback remain mandatory. TCP/raw REALITY creation now prepares the corrected core as well as XHTTP. TCP upgrades preserve the installed panel binary; Fragment requirements retain their existing compatible panel. Core artifacts are available for linux/amd64 and linux/arm64.

## Operational evidence

- The six reviewed jobs were cancelled using the existing transportapply.Rollback operation, under executor/config locks and a fresh empty native snapshot. Every result was FAILED / CANCELLED_BEFORE_WRITE. This proves release of the blocking jobs; it does not establish what originally removed the native inbounds.
- Subsequent ordinary reconciliation restored ALL output coverage of the stable healthy cohort in the sampled 20:25 UTC window. Some Residential profiles were still filling; final coverage acceptance is pending.
- First resource canary (dob-fm-0.2.0), installed at 20:41 UTC on 172.237.8.215, passed config and native identity preservation. At uptime 112 seconds: available memory 953,220 KiB, Xray swap zero, statistics response 42 ms; a nearby profile counted six pipe descriptors. The pre-installation sample had roughly 54 MiB available, about 307 MiB of Xray swap and over 21,000 pipe descriptors. Restart also releases old connections; sustained observation is required before attributing all improvement to the correction.
- The intermediate native candidate dob-fm-0.2.1 addressed both defects and is published with complete modified source, patches, licenses, dependency locks and SHA-256 manifests at artifact revision 7c587e6952e17dbd967aaade3f6be4dd6a3b9da2. This intermediate revision was superseded by dob-fm-0.2.2 after the concurrency follow-up below.
- The operator protection policy observed in this turn was OFF revision 51. No policy toggle, health override, freshness relaxation, provider deletion, or model API call was used.
- The historical retained test panel 21727ecc-3871-4c7b-9653-f1db84b81d44 was already recorded DELETED when checked in this turn, with an earlier 13:04 UTC expiry. This task did not delete it and did not treat that historical record as a usable canary.

## Validation

- The pre-write retry regression failed on the original source with BLOCKED instead of FAILED. The final ten-case database test covers pre-write/manual retry, saved backup, unknown native writes, rollback source/parent and inconsistent markers. Transport application, global listener reconciliation and client mutation race suites passed.
- A complete serial database-integrated Go suite passed for the transport retry correction. Follow-up runtime preparation changes passed focused transportcore/desired/transportapply/globalreality race suites. These are separate checks, not a claim that the earlier full run used the final native artifact manifest.
- REALITY native tests passed under the race detector three times: idle closure, bounded buffers without ReaderFrom/WriterTo, active one-way transfer, half-close response, cancellation of rate-limit waiting, stop/expiry races, interrupted handshake writes and safe ownership handoff.
- Xray VLESS/encoding, REALITY and FinalMask compatibility tests passed (ordinary tests, despite the historical evidence filename containing race). Linux amd64/arm64 builds passed using Go 1.27.1. go vet passed for product source.
- Early isolated-test setup failures and one invocation naming a nonexistent package were retained separately; neither is counted as successful verification or as the product regression.
- Completed task-owned PostgreSQL fixtures were removed only after test clients closed; production databases and backups were untouched.

## Acceptance scope

Final acceptance below records the live canary receipts, sustained resources, source/public revisions, both-service-host installation, native fleet audit and actual public subscription coverage. No permanent all-green or upstream-availability guarantee is claimed.

## Bounded concurrency follow-up and final native candidate

The v0.2.1 canary removed pipe retention (10 pipe descriptors instead of more than 10,000 on the 1 GiB host), but the 45.33.68.241 sample still retained about 3,500 simultaneous invalid fallback connections and suffered real memory pressure. Timeouts alone were therefore insufficient. The final dob-fm-0.2.2 core additionally caps unauthenticated fallback forwarding at 512 connections per process. A connection acquires one shared slot across both directions, never waits for capacity, and releases its slot after both tasks join, including timeout paths. Authenticated REALITY handshakes/transport do not consume fallback slots. Expired/refused raw TCP sockets use zero linger where supported so queued retransmission buffers are aborted rather than retained as orphans. These bounds apply inside the protocol implementation; no server-guardian policy was enabled or changed.

The additional race tests verify admission saturation, no double reservation, release after expiry, slot reuse and abort-only-on-expiry behavior. Both architecture builds passed. Complete final native artifact: eafb3c6ce3db743ec2350d94249f6cf05cf9ec46, branch transport-runtime-v0.2.2. Earlier v0.2.0/v0.2.1 canaries and artifacts are retained as historical observations; they are not represented as the final candidate.

All three selected canaries installed v0.2.2 successfully between 21:00:30 and 21:00:50 UTC with config validation, actual running-image checks and native identity preservation. Busy initial configuration locks were skipped and then acquired in a bounded subsequent attempt before mutation. The final pre-write SQL guard also blocks noncanonical/changed saved JSON for review instead of letting that job indefinitely occupy the front of the executor queue; the tenth regression case covers an unknown saved source field.

Disk preflight refused publication/build reserves without stopping services. Completed isolated test fixtures and 35 redundant cached candidate archives from clean, published source clones were reclaimed. A later reserve check also reclaimed 35 redundant clean, published publisher-cache clones. Exports, validation/installation logs, current and rollback releases, canonical source history, production databases and backups were retained. Both isolated test fixtures were removed after all test clients closed. Installer reserve checks passed before stopping services.

Final v0.2.2 canary resource sample at 71–89 seconds uptime: 45.33.68.241 available memory 343,180 KiB / stats 69 ms; 172.236.162.223 available 503,292 KiB / stats 93 ms; 172.237.8.215 available 1,266,688 KiB / stats 66 ms. Recent memory PSI was zero or near zero. These are measured post-installation samples, not a permanent availability guarantee.


## Product release and fleet installation

Product source 294bd152633e32014e7ce3bb2649fa89f78e4e89 was published as public revision 2471b22a90dead5468d3ed1a2acc9dc7d16a0f71. Main/stable CI runs 38967109693 and 38967109473 succeeded. Both service hosts installed this revision: secondary verified at 21:10:54 UTC and primary at 21:12:40 UTC. All three services on each host were active, their running hashes matched, automatic restart counts were zero, and each host passed five readiness observations. Schema remained 000182_fleet_client_coverage. Account/environment hashes and creation, output and protection policies were preserved. All 319 retained primary listener pins were unchanged.

The two-worker native rollout completed 78 eligible non-canary panels: wave 1 verified 50 and skipped 28 busy configuration locks; wave 2 acquired those locks with bounded waiting and verified all 28 without errors. Together with the three canaries, 81 distinct selected panels had validated installation receipts. Current-cohort running-image audit results follow separately, including nodes created during rollout. No native identities or client credentials were rewritten by the upgrade.

At 1,279–1,298 seconds of uninterrupted final-core uptime, the three resource canaries retained 410,460 / 525,216 / 1,325,680 KiB available memory (hosts 45.33.68.241 / 172.236.162.223 / 172.237.8.215). Xray swap was zero; memory PSI full avg10 and avg60 were zero; statistics calls completed in 59 / 81 / 57 ms. Earlier immediate restart samples are not used alone as evidence of sustained recovery.

Evidence: primary-product-verify.json, secondary-product-verify.json, fleet-native-wave1-progress.json, fleet-native-wave2-progress.json and native-v022-resources-post-fleet.json under the recorded evidence root.


An independent read-only SSH audit verified all 85 current eligible panels: on-disk and running executable SHA-256 values matched the immutable architecture-specific manifest, bounded fallback and handshake capabilities were present, and the fallback limit was exactly 512. There were no mismatches or SSH audit failures. This cohort included four newly created panels outside the reviewed 81-panel installation set; each had already acquired dob-fm-0.2.2 through normal product provisioning. This validates the future-node integration as well as the one-time fleet upgrade. Receipts: fleet-native-final-audit.json, fleet-native-final-summary.json, fleet-native-new-nodes.json.


## Subscription observation windows

The first ten post-rollout samples preserved startup and refresh transitions rather than filtering them out. A short provider-error observation recovered on the next inspection. Two newly provisioned panels initially had no published snapshot; normal reconciliation subsequently admitted them. Intermittent route APPLYING and one stale inventory observation also recovered. These observations do not support a claim of perpetual, instantaneous all-green counts during fleet churn.

In the subsequent six-sample window (21:28:22–21:30:56 UTC), both authenticated ALL output and the existing public ALL share covered every server healthy immediately before and after each sample. All six samples had zero missing healthy hosts and zero invalid configuration links. Dashboard degraded count was zero throughout. Direct coverage also converged to the full healthy cohort. Newly arriving Residential profiles continued filling under the unchanged 60-second creation interval. Two bounded uses of the standard user-capacity reconciler retained execution gates, operation budgets, policy cadence and configuration locks; no direct client-table or native-configuration edits were used. Final per-profile results follow below.

The two later arrivals also passed independent on-disk/running-image hash and capability checks, bringing distinct verified native panels to 87 across the changing fleet. This is a cumulative verified count, not a claim that all 87 remained simultaneously active. Evidence: fleet-native-latest-arrivals.json.


## Final acceptance record

Observed at 2026-10-16T21:33:59.710870+00:00. Stable healthy servers: 85. Health reasons: {"HEALTHY": 85, "PENDING_DELETION": 5, "PROVISIONING": 3}. Authenticated and existing public-share results are recorded in `docs/development-jobs/route-pressure-20261016.acceptance.json`.

- DIRECT: 85 servers, 85 links, 0 missing healthy hosts, 0 below profile target, 0 invalid links.
- RESIDENTIAL: 85 servers, 255 links, 0 missing healthy hosts, 0 below profile target, 0 invalid links.
- ALL: 85 servers, 425 links, 0 missing healthy hosts, 0 below profile target, 0 invalid links.
- SHARE_ALL: 85 servers, 425 links, 0 missing healthy hosts, 0 below profile target, 0 invalid links.
- SHARE_DIRECT: 85 servers, 85 links, 0 missing healthy hosts, 0 below profile target, 0 invalid links.
- SHARE_RESIDENTIAL: 85 servers, 255 links, 0 missing healthy hosts, 0 below profile target, 0 invalid links.

Both confirmed blocking/resource causes are corrected in installed product code and the pinned native core. Creation and expiry continue normally; provisioning and initial verification can still produce temporary inactive counts. The ALL subscription passed the six-sample complete-coverage window and final terminal checks. No health flags or freshness gates were relaxed. The final-source focused race checks, native race/compatibility tests, architecture builds, publication gates and both CI runs passed. Model API usage remained zero.

A final transient provider-network observation affected ten dashboard server entries: one Vultr account reported PROXY_ERROR at 21:33:31 UTC while its network identity was rotating. The normal recovery path restored ACTIVE / READY with no provider error by 21:33:48 UTC. The final 21:33:59 UTC sample had no provider-unavailable servers. All three public output classes retained coverage during that observation. This real transient was retained, not cleared manually or concealed by a health override.
