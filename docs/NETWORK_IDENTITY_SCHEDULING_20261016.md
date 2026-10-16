# Network identity admission starvation — 2026-10-16

Status: reproduced; new plan review blocked by exhausted model API credit. No production source change or deployment for this follow-up.

## Evidence

The output/proof/guardian correction is deployed as public124f51f/sourcee4e1e544 on both hosts. Its output acceptance is separate and retained. During longer observation, provider errors returned even with healthy base proxies. Economy policy is enabled globally. Account identity observations aged205..228seconds while the fail-closed guard permits180seconds. Control heartbeat showed parent3 with2active, bulk1 occupied with7waiting, and no SQL pool wait.

Current network-identity work shares controlBulkWork with lifecycle, deletion and other long management jobs. It creates its45second operation deadline before acquiring that admission class and waits for a whole goroutine batch. The regression runs the exact production control-budget source, verifies actual main wiring, holds unrelated bulk work and observes the identity operation time out while two parent permits remain unused. The test deadline is scaled to45milliseconds; live evidence supplies the actual prolonged delay. A separate-class mechanism demonstration passes while retaining total parent3, but this does not qualify a production scheduler, fleet capacity or behavior under all competing workloads.

## Concrete corrective plan requiring review

1. Give identity maintenance its own bounded admission class and completion-driven fair discovery, within an audited aggregate control/SQL budget. Final concurrency must be validated against provider2, long bulk work, account leases and the24-connection control pool.
2. Create the45second operation timeout after admission. Never hold SQL or account ownership while waiting for a work permit. Join cancellation before returning permits.
3. Prioritize old/due identities and avoid a whole-round barrier. Cover10accounts, one slow/failed/rotating account and concurrent long management/provider tasks. Preserve transaction-scoped proxy-keeper ownership and authoritative rechecks.
4. Keep180second identity and360second base freshness, proxy-required routing, collision/country policies, retry/circuit limits, credentials and existing sessions unchanged. Do not disable economy or fake verification timestamps to remove warnings.
5. Require baseline-to-fixed, cancellation/fairness, real PostgreSQL pool/ownership, full regression and independent critical council gates; then normal publisher, both-host canary activation and repeated live age/provider acceptance.

The last external model request failed credit_balance_exhausted. No new calls were made in this continuation. The previous council cannot approve new code it has never seen. Project rules require architect/reviewer/adversary/test-design review for this critical change. Resume after API availability is restored, or an explicit user exception authorizes work without that council; do not silently reinterpret the prior three-side-defect exception as applying here.
