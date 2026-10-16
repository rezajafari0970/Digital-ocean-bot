# Fleet identity and client coverage repair

The user explicitly authorized this repair without OpenAI API. No external model review is claimed and no API request was made. Reproducible evidence remains the release authority.

## Proven causes

1. Identity maintenance shared bulk1 with slow management and started its45sdeadline before admission. Reproduction expired with2parent permits idle. New bounded identity2 class and provider2 plusbulk1 reserve independent progress in aggregate5. Two completion-driven workers prevent whole-batch head-of-line blocking, close discovery rows before acquiring account ownership, start45swork deadline after admission, and join cancellation before releasing permits. Freshness rules and per-account ownership remain unchanged.
2. Live92READYpanels had DIRECTownership64 and Residential3ownership62. Missing panels had no lifecycle scope and no pending/failed job, so UI misleadingly showed0waiting/0review. Global auto-enrollment cap64 was the cause. Migration182 upgrades only historical default64 in an already-enabled unscoped global lane to0(all eligible). Custom caps,paused gates,kill switches preserved. New installations defaultall eligible. Client mutation concurrency remains1; per-scope finite budgets/expiry,ownership,quarantine,routeproof and journals remain.

## Capacity and validation

Aggregate controlwork changes3to5 without enlarging the24connection pool. Pessimistic long-pin qualification:bulk6+provider2each+identity3each+role1=17; each of5owners still obtains one further SQLconnection while supervision reads proceed, with zeroSQLwaits. Targeted actualPostgreSQL/race tests cover these constraints, slow peer and9healthyaccounts, admission delay, cancellation join, discovery backoff,96enrolledpanels,kill/locked/expiry/custom-cap guards and migration preservation.

The operator panel now reports scope coverage,eligible/enrolled ports,awaiting enrollment andpaused scopes. URI-only public subscriptions remain unchanged. Old source rollback can continue serving existing users; restore a positive scope cap before rollback if new automatic enrollment must continue.

## Resource protection evidence

Video and synchronized live samples distinguished actual pauses from overdue reports:92/92fresh,0receipt errors,21–26paused before the external policy change. The desired policy becameOFFrevision45 at08:19UTC; this task did not disable it. Respect that setting. Read-only SSH process sampling on five servers showedXraydominant CPU where pressure remained, including~46%CPU/~445MiBRSS onone1vCPUserver; other sampledhosts1–7%CPU atthatmoment. This does not establish monitoring overhead as the cause and is not proof that past pressure was false. Kernel conntrack pressure was also present in retained receipts. Do not erase genuine pressure thresholds or force all hosts healthy.

Three cleanup observations subsequently had SSH timeout/reset errors; bounded retry and precise per-node verification remain required. Newly provisioned/retiring servers and changing fleet counts must be distinguished from failed existing servers. Never delete temporarytesthost104.105.133.93.

## Deployment status

Pending exact-source full suite, normal publication, staged both-host activation and repeated live profile/identity/resource acceptance. Installed baselinepublic124f51f/sourcee4e1e544. No Xraycoreorserverguardianbinary change in this candidate. See jobJSON and videoledger for tracked evidence.

## Populated upgrade correction — 2026-10-16T08:58:51.169961+00:00

Firstpublic4d156e passedbothCIandsecondaryactivation; primaryinstaller correctly rolled back to124f51f when PreserveData rejected the intended scope64to0policy transition. No data/retention/checksum guard was disabled. Added a populated retained-upgrade regression: it reproduced the identical error on baseline. The existing per-version expected-transformation mechanism (previously used for172) now declares only migration182bulk_lifecycle_control max64to0andits transactiontimestamp under the exact enabled/auto/global/unscoped/gate predicate. Everyothercolumn,row,budget andgate is retained. A negative regression proves an unrelated operation_budget overwrite is rejected and rolled back. The immutable182migration bytes/checksum remain unchanged; an already-upgraded secondary skips it normally. Targeted retained/race,full andvet gates PASS. New publication and bothhost verification pending. APIcalls0.
