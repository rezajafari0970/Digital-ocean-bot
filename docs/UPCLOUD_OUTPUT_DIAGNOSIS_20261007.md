# UpCloud output diagnosis — 2026-10-07
Status: DIAGNOSED_NOT_FIXED. Read-only production investigation; runtime remainsfc97105.

This supersedes any reading of the earlier live acceptance as current serving readiness. User asked to find why no configs appear in Output, not to change settings.

## Confirmed cause
Upcloud113 account63cf71fa-ad4b-4ee0-a72b-b3e3cad22fbe has desired5, provider capacity2 and both slots occupied. Both servers expired on Oct6 at22:48:19Z/22:53:46Z (Oct7 at02:18:19/02:23:46 Tehran), over5 hours before this read. Oldest47fc5... is EXPIRING; second7083... remains READY despite expired time. Neither has a replacement deployment or DELETE operation; both have a valid profile.

internal/app/lifecycle.go calls shouldWaitForDeficitBackfill before capacity.Read. In internal/app/lifecycle_capacity_policy.go this returns true when managed<desired, hence5/2 causes an early successful return. The capacity-full retirement branch below it is never reached. internal/droplets/lifecycle.go chooses only one item per account, prioritizing EXPIRING, so this stalled oldest item also starves the second expired READY server. No error is returned, so worker failure telemetry stays empty. Heartbeats and scheduler continue.

internal/app/deploy.go requires runtime READY and available provider capacity, both unavailable while the two slots remain occupied. This is a circular wait: deletion waits for a build, build waits for deletion. Provider billing denial or trial expiry is not required to explain it.

## Output and native panel evidence
Native Sanaei API still sees11 clients on e8b83162 (2 enabled,9 expired) and1 client on e83bed21 (enabled, not individually expired). There is a443 inbound on each. This is not evidence that all clients are usable: server expiry overrides individual client expiry.
Output correctly requires droplet expiry later than now+10s and removes stale/ineligible snapshot rows. UpCloud has zero snapshot rows. Other accounts had about58 rows passing the exact ALL predicate in one fresh read; aggregate counts rotate continuously.
Separately,11 RESIDENTIAL routes are effectiveBLOCKED with zero trial-compatible healthy proxies;1 DIRECT route exists. Residential output is independently withheld, but it does not explain why every link from an expired server is absent.

## Required bounded fix (not applied)
Resolve effective capacity before the deficit wait. At fresh confirmed full provider capacity, serialize retirement of one oldest expired owned server, confirm provider deletion, then backfill at configured cadence without changing Desired5. Preserve unknown-outcome reconciliation, account network identity, create-denial guards and non-expired serving capacity. Add a real regression covering desired5/limit2/managed2/pending0, per-account queue progress, concurrent workers and successful refill. Test expiry/rotation, not only initial panel readiness. Do not disable Output expiry filters or publish blocked residential links.

Evidence: /root/backups/dob-upcloud-output-diagnosis-20261007/facts.json and native-panels.jsonl. Server OpenAI source review confirmed the causal path. No product source, production data, services, Desired value, proxies, safety controls or account permissions were changed in this diagnosis.
