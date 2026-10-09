# Residential selection tuning — 2026-10-09

The measured issue is slow and intermittently failing residential static-asset transfers. The current profile requests 13 fast proxies while only 12 are configured: native Xray has identical fast and all selection counts (12). A second VPS also has approximately 235 ms raw TCP latency from the control host, so transport selection cannot make every real HTTPS probe less than 10 ms.

## Supported change

A timed selected trial stays inside the current permanent fleet profile. Only fast_count and fast_share can change. Untargeted and newly enrolled servers keep the saved parent specification until publication. The reviewed operational candidate copies every parent field and changes 13/80 to 3/90. The ordinary start busy guard and single-open-profile index are unchanged.

Store.Do owns tune_start, tune_cancel, tune_publish and tune_restore. Operator actions require the parent version and intended tuning ID; operation receipts retain historical replay semantics. Read current status after replay. The admin API retains its existing principal check. The root/service-environment operator command accepts one bounded JSON request and invokes Store.Do only.

Automatic expiry and failure rejection persist recovery intent in a transaction separate from ordinary enrollment. Restoration changes are atomic under a savepoint; conflicts leave RESTORING and preserve evidence. Missing target recovery records pause enrollment/rollback instead of fabricating before_config. Surviving disabled, expired or unreachable targets remain obligations; genuinely deleted inventory is exempt.

PUBLISHING is distinct from PUBLISHED. Completion requires current tuning identity, expected generation, configuration ownership and fresh routing/runtime acknowledgements for the full required cohort. Selected RESTORED means the selected trial obligations were restored, not that unrelated fleet assignments were repaired. The latest publication is the only tuning restore point; starting a subsequent settled trial supersedes it. Preserve the exact previous specification in operational evidence.

## Native proof and lock order

The native worker holds the cross-process panel-config lock, then its runtime mutation lock, and takes the performance advisory transaction lock only for acknowledgement. Store actions never wait for panel locks. A delayed old native operation can finish before restoration; its stale acknowledgement is rejected. It cannot execute after another worker's restoration acknowledgement while both follow the same panel lock. Subsequent reconciliation applies the current generation.

Reserved dob-route/residential-ads-generated routes are worker-owned. Complete native settings, including pool selectors, enter the content-addressed plan fingerprint. Even when there are no client inbound tags, hardened internal DNS and applicable pool routes must prove the loaded plan. Process-running state alone is insufficient.

## Deployment and rollback

Deploy all three services together and verify exact executable hashes. The normal API migration runner applies additive migration 165 before panel workers start. Existing independently supervised residential-performance reconciliation runs every five seconds; successful physical recovery is not guaranteed for unreachable servers.

Deployment holds /opt/.digital-ocean-bot-deploy.lock exclusively; the operator tuning CLI holds it shared. Old API and workers are stopped before rollback guards. Active TESTING, PUBLISHING or RESTORING prevents deployment of an overlay-unaware old runtime. Restore and verify assignments first. Migration down also rejects active obligations; preserve a settled publication recovery record before deliberately removing it. Keep runtime rollback assets separate from exact database/profile recovery evidence.

## Acceptance limits

Use bounded public static JavaScript downloads, truthful direct VPS probes, and real VLESS/Reality paths. Keep every failure and denominator. The predeclared comparison includes sequential and two-concurrent transfers, median and nearest-rank p90, and a minimum improvement threshold. Publication is not authorized by passing unit tests alone. Do not generate live ad requests or impressions for benchmarking.

These measurements do not establish mobile app show-rate, creative quality or Android latency. Those require the actual client network and SDK load/show/impression callbacks. Runtime outcome and final status are recorded separately after the canary.
