# Proxy separation and consumption audit — 2026-10-08

Verdict: no account-base/residential cross-route observed in the verified scope. This is NOT a complete fleet or historical no-misuse certificate. Consumption control and attribution gaps remain. No production configuration, credential, gate, lifetime, quota, route or binary changed.

Audited source HEAD: 2c2f904f53903d4e3ec55a5a9b09f2ec8904c68a; deployed product: bf2d6daba2b963e66296a38fe3e62304dd90de4a.

## Observed separation
- One DataImpulse account-base record and twelve configured residential records, each differing from that base record: no matching configured host, username, password or authenticated identity. Separate DB/secret scopes; all twelve current residential credentials use residential AAD. Provider-side subscription/billing/quota independence was not verified.
- Nine account profiles (five enabled) use that one base identity using that same base record. This is shared configured provider identity/likely quota among accounts, not evidence of residential sharing; separate ports do not establish separate billing pools.
- Attempted all81 non-DELETED enabled panel records: eight readable (six DO, two UpCloud),73 Vultr login/read unavailable. No DataImpulse endpoint/credential and no unknown proxy endpoint in the eight.
- 72 successful native read-only route evaluations: Direct class remains direct; residential Google-ads TCP/UDP uses pool lanes and second-stage residential outbounds; ordinary Google web route is direct. The two observed UpCloud configurations block protected ads while preserving Direct/general routing. These are route-evaluator results, not end-to-end payload tests.
- Case-insensitive /proc/PID/environ snapshots for API/control/panels/browser-manager found no HTTP_PROXY/HTTPS_PROXY/ALL_PROXY or lowercase equivalents; the final collector also checked NO_PROXY/no_proxy names without recording values. These snapshots are not end-to-end transport traces. A listener inventory was captured; listener inspection alone does not exclude application-layer relays or external credential use.

## Confirmed risks and candidates
### PSA-001 — HIGH — Expired cloud nodes remain running with enabled unlimited clients and increasing inbound counters
All eight reachable panels report Xray running, DB server expiry passed, four enabled clients with no volume/expiry limit each. Between two reads aggregate inbound counters increased 1,022,471,317 bytes. The authentication outage blocks proxy-required provider operations and may block provider-side cleanup; this audit did not trace a deletion attempt for each of these eight nodes. Remote Xray remains running independently.

Limit: Inbound bytes include Direct and residential-class traffic; they do not prove billed residential bytes, DataImpulse bytes, outsider misuse, or amount since expiry. Lifetime=0 is existing intentional policy; do not reset it silently.

Next: Design idempotent panel-local retirement/expiry enforcement independent of provider deletion, preserving configured client policy and existing ownership/gates; do not blind-delete or bypass provider restrictions.

### PSA-002 — MEDIUM — Recurring residential probes multiply by panel count
Six reachable DO panels each select all12 residential outbounds with burstObservatory interval10s, sampling1; central monitor schedules enabled healthy targets every30s. Conditional calculation only: IF Xray probes each subject once per configured10s interval, six panels times12 subjects times8640 gives622080 attempts/day; IF all12 central targets remain enabled and healthy,12 times2880 gives34560 checks/day. Exact Xray per-subject scheduling and completed attempts were not measured.

Limit: Not measured completed requests or billed bytes. Proxy health needs panel-local reachability; global health is not a safe wholesale substitute. UpCloud panels have no residential outbounds/probes in the observed configuration; this finding does not assign a cause.

Next: Measure outbound probe bytes, use bounded adaptive cadence and jitter, and retain per-panel fail-closed recovery.

### PSA-003 — HIGH — No reliable proxy-byte attribution for this incident
All eight readable Xray settings enable inbound byte stats but disable outbound byte stats. Control-plane health records have latency/check counts, not per-proxy bytes. Provider usage export was not available.

Limit: Current auth failure happens before destination transfer and cannot explain the user-reported rapid quota depletion by itself. Stored provider snapshot size is not transferred or billed network size.

Next: Add separately scoped accounting for base and residential outbounds; reconcile provider usage by timestamp, source IP and subuser without logging secrets.

### PSA-004 — MEDIUM — Separate storage does not prevent future duplicate endpoint credentials across roles
Current base and each of the12 residential records differ in host, username and password. Reviewed application write paths are base create/update, residential single create/update and residential bulk import; none contains cross-role identity exclusion. Current schema constraints/indexes/triggers are captured and contain no cross-role authenticated-identity exclusion. Direct operator SQL and every historical migration writer were not exhaustively audited.

Limit: This is a future-configuration prevention gap, not evidence of current mixed routing. Shared gateway IP alone is not proof of shared billing identity.

Next: Define canonical authenticated-provider identity and reject cross-role reuse in every write path plus runtime validation; test same host with distinct legitimate subscriptions and concurrent changes.

### PSA-005 — MEDIUM — Provider observation repeats catalog requests; byte cost is unmeasured
DO refresh becomes eligible when its stored snapshot is older than60s. Driver.Observe calls GetAccount, Catalog and ListDropletModels. Catalog itself calls GetAccount again and requests regions, sizes and distribution images. Thus a successful full observation includes a duplicate account read and refreshed catalog; it does not call legacy Discover or fetch projects/keys/firewalls through that path. UpCloud/Vultr use fastObserve with reused catalog. The two DO account profiles are proxy_required on DataImpulse. Current auth rejection prevents successful destination transfer; historical request/byte overhead is not measured. Base monitor configured10s; account identity checks separately scheduled.

Limit: No historical billed byte counter; frequency is source/configuration and can be constrained by admission/backoff. Do not infer amount from snapshot payload size.

Next: Measure requests/bytes and separate catalog TTL from frequently refreshed resource inventory.

### PSA-006 — MEDIUM — Panel HTTP transport can inherit future proxy environment settings
Sanaei API client falls back to http.DefaultTransport. Current API/control/panels/browser-manager processes have no HTTP_PROXY, HTTPS_PROXY or ALL_PROXY variables. Provider gateway has an explicit isolated transport and residential uses its own secrets.

Limit: No HTTP/HTTPS/ALL proxy variable was found in the case-insensitive process-environment snapshots. HTTP proxy variables do not control the separate direct SSH net.Dialer.

Next: Use an explicit panel-management transport with Proxy:nil and a regression test; preserve TLS/timeouts.

## Current incident and limits
The user reports unusually rapid depletion of proxy allowance. This is the incident being investigated; no provider billing/usage export independently verifying the depletion was available.
DataImpulse last observed healthy at02:35:31UTC; current proxy authentication is rejected. This cannot by itself distinguish wrong credentials, exhausted quota or a provider restriction. No cause is assigned without provider evidence.

All eight reachable panels still run Xray with32 enabled unlimited clients despite expired cloud-server records. Native inbound counter growth between the09:01:19-start audit and09:09:39–40 followup totals **1,022,471,317 bytes**. This is all inbound client traffic, not measured residential or DataImpulse consumption and not proof of unauthorized clients. Individual native sampling times differ slightly.

The six observed residential panels configure72 subjects with interval10s. Conditional arithmetic gives622,080/day ONLY if every subject runs once each interval; exact Xray execution semantics were not established. Central34,560/day additionally assumes all12 targets stay enabled and healthy. Neither figure is an observed request count, byte measurement, provider charge or a verified upper bound.

73 unreachable Vultr records remain unverified. Their existence does not prove73 live servers. No provider usage export or historical external-use log was available. There is no evidence sufficient to confirm or exclude stolen-credential use.

## Verification and continuation
Focused race tests passed for internal/network, internal/secrets and internal/panels/residentialsync; production DB was excluded. PostgreSQL-specific tests were skipped without an isolated test DSN. No product implementation or deployment is claimed.

Priority: separately scoped byte accounting; policy-preserving retirement independent of provider connectivity; adaptive measured probe overhead; cross-role identity exclusion at writes/runtime; explicit direct panel transport; then provider discovery efficiency. Correlate provider usage timestamps/source IPs/subusers before attributing the user-reported base-proxy consumption incident.

Evidence directory: /root/backups/dob-proxy-separation-audit-20261008. Sanitized native settings/route results are in native.jsonl, followup.jsonl and routes-final.jsonl. Audit helpers are read-only DB/native readers under .local/proxy-separation-*. Full structured findings: PROXY_SEPARATION_AUDIT_20261008.json.

## Deployment and verification provenance
The source HEAD differs from the deployed bf2d6da revision only in nine documentation files; no product-source diff. Actual API/control/panels binaries report bf2d6da and vcs.modified=false, original PIDs2739199/2739201/2739202, active with zero restarts. Browser manager remains its older5d68621 build; only its live environment/listener observations are attributed to that process. Current schema constraints/indexes/triggers, per-panel expiries, native Xray/client/route evidence and exact successful race-test argv/exit0 are embedded in the JSON report. Test environment excludes production DATABASE_URL and BULK_TEST_DATABASE_URL.

Final evidence assembly: 2026-10-08T09:37:22.933696+00:00. Native traffic snapshots remain09:01/09:09UTC; later schema and health observations are separately timestamped in JSON. The credential-store source and live credential-table constraints/indexes/triggers are also in the review package.

Final report regeneration: 2026-10-08T09:43:01.445286+00:00. Findings based on native inbound counters do not claim successful end-to-end forwarding.

Review record: PROXY_SEPARATION_REVIEW_20261008.json. Orchestrator audit workflow COMPLETE; final actual API review PASS is limited to closure of identified report revisions. No product fix or no-misuse certificate is implied.
