# Bounded per-VPS residential admission

This capability adds an evidence-authorized, timed exclusion trial to the existing durable performance tuning controller. It is not permanent or automatic fleet quarantine. The latest operational outcome and exact release commit belong in the accompanying status report and HANDOFF.

## Contract

- One panel, one or two excluded proxy UUIDs, two distinct successful controls, 5–30 minutes. All other configuration fields must equal the durable parent. The first operational pilot is five minutes with the existing 13/80 parent.
- A trusted operator collects three scheduled attempts at each of four fixed HTTPS targets: gstatic204, BrowserLeaks, AdsJS and GPTJS. HEAD only, valid TLS, no redirects, no retries, at most three concurrent requests and a six-minute overall context. The oldest observation must be at most five minutes old at admission. Every failure and unstarted slot remains in the receipt.
- The diagnostic path is control host → fresh panel-owned DIRECT VLESS Reality client → one residential SOCKS gateway → destination. It measures that VPS-to-gateway path; it is separate from native pool selection, mobile v2rayNG latency and ad impressions/show-rate.
- An additional HTTP204 positive/blackholed-dialer guard proves chain necessity. Both guard outcomes are retained. The reviewed negative signatures are curl52/56, no HTTP response, transport failure, and an available negative-core listener. HTTP errors, TLS failures, local connection failures and unknown signatures do not authorize admission.
- Each suspect must fail the same Ads or GPT target in all three rounds. Both controls must pass every matrix attempt and remain enabled/currently healthy. Generic central health alone never authorizes an exclusion.
- Evidence binds the panel/API identity, routing revision, native plan, parent owner, performance generation and monotonically increasing endpoint/credential versions. Evidence content is immutable. Only the latest receipt for that panel may authorize a new operation; recording and authorization share the existing transaction lock. Unexpected diagnostic-core exit and local listener connection failure are non-actionable. One receipt can authorize one operation; exact committed request replay retains its existing receipt.
- A single copied effective proxy list feeds TCP/UDP candidates, observer, planning and native acceptance. No excluded outbound remains in another lane. Existing residential allowlist, direct probes/DNS exceptions, fail-closed behavior and unrelated DIRECT clients retain their contracts.
- The controller binds the candidate plan hash transactionally before native mutation. Unrelated plan changes, source changes, expiration, cancellation and current-generation verification failure trigger durable restoration. Ownership conflicts remain visible and pending; they never overwrite a newer owner.
- The existing cross-process panel lock serializes native writers. An in-flight candidate may finish while restoration is pending; stale-generation ACK is refused, and restoration is serialized afterward. An API/network outage can delay physical restoration beyond the deadline. RESTORING is never reported as RESTORED until native proof is fresh.
- Admission cannot use permanent start or tune_publish. It cannot rewrite global proxy enablement, change the permanent fleet profile, extend server lifetime, or change allocation/spending policy.

## Operator interface

Build `cmd/residential-admission`, `cmd/residential-performance-tune` and `cmd/residential-routing-acceptance` from the reviewed release. Use the protected service environment on the controller host. Never print credentials, URI values, source config or environment files. No managed-panel SSH is required or permitted by this workflow.

`residential-admission -probe -panel PANEL -suspects UUID[,UUID] -controls UUID,UUID -xray /absolute/reviewed/xray` saves a safe receipt. `-evidence UUID` reads it without network probes. Startup failures and interrupted matrices are ineligible, not substituted by successful retries. The probe never changes panel routing.

Construct and save one exact `residential-performance-tune --execute` stdin request using `tune_start`, the current parent/version, one panel, exact before configuration plus `excluded_proxy_ids`, matching `admission_evidence_id`, fresh base revision/plan and the reviewed duration. Preserve its request UUID for ambiguous-result replay. The Store enforces eligibility again at commit. Do not regenerate request parameters when replaying.

Native acceptance and real RES traffic checks are independent gates. Cancel using the supported lifecycle request if scope, generation, native routes, allowlist or predefined reliability checks fail. Let the first trial expire automatically if those gates pass; verify exact parent configuration and native routing afterward. No automatic extension or publication is supported.

## Compatibility and storage

Migration166 is additive. Endpoint and encrypted-credential mutations advance admission versions; central health observations do not. Credential reassignment advances both old and new endpoint identities. Receipts cascade only with deleted inventory and are otherwise append-only. Operation evidence IDs are unique.

Supported deployments use the shared exclusive host deploy lock and stop all readers before publishing. The staged manifest declares admission schema1. The normal compatibility entrypoint refuses older consumers with active recovery or any still-applied exclusion, including orphaned assignments. The rollback path also refuses unresolved tuning. Migration downgrade refuses active admission. Finish restoration and native verification with the capable runtime before returning to runtime98bcb7e.

## Evidence boundaries

Baseline source: ad1b0df0a0c7e4fc66fd1c8b77503790a26e2b27. Runtime rollback:98bcb7e043cd982a8a5e01070b6fcf7080390d69. Engineering evidence resides under the scoped admission job, with isolated PostgreSQL tests, race/fault tests, actual Xray26.3.27 and26.9.9 tests, independent reviews, complete regression and controlled release evidence. The historical detached canonical bootstrap failure remains recorded; this capability does not assert repository-wide READY.

Automatic repeated quarantine/readmission, long-term destination health scoring, mobile latency verification and app SDK show-rate instrumentation are subsequent phases. Static public-resource success must not be reported as ad fill, impression quality or improved show-rate.
