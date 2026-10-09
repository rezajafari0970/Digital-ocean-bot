# Authorized residential categories and observed health

User authorization: 2026-10-09 23:42 Tehran. Preserve Google ads and BrowserLeaks and add exactly the ten requested category tokens. Explicitly includes full matched YouTube/Google Play payload, not merely their ads. More proxy bandwidth is expected; no 90% reduction claim.

`DOB_RESIDENTIAL_CATEGORIES_PANELS` is a deployment selector: empty, none or malformed keeps the previous category list; exact UUIDs stage a canary; all includes current and future panels. The worker and native acceptance must use the same selector. Roll back with none using the current compatible release and verify restored native routes. Strict allowlist remains enabled; this is not unrestricted direct fallback.

Local installed geodata accepts all requested tokens, but huawei@ads and gog@ads have no entries. Add only domain:dt.dbankcloud.ru and domain:insights-collector.gog.com, the attributed domains in official v2fly/domain-list-community commit c6adacf29cc4fb05e04651fcf03e00989ef9f797. Do not silently update global geodata or broaden to huawei.com/gog.com. Native representative proofs are required because a parse-only success does not prove attribute content.

Public and pool-inner TCP/UDP rules share the selected domain contract. Infrastructure probe/DNS/DoH exceptions, explicit DIRECT identities, deny-default, unknown users, UDP capability gating and private-IP protection are preserved. Geosite routing is domain-based; it does not recognize SDK operations, URL paths, or force opaque requests to match.

`residential-health-status -panel UUID` reads the latest stored diagnostic receipt and current context in a read-only repeatable-read transaction. It makes no new probe, cannot authorize mutation, and reports stale/invalid/context-changed evidence explicitly. A successful observation is not provider reliability, native pool or application success. No report coverage exists for unmeasured category targets. The existing collector still has its fixed four-target manifest; old NO_ACTION is unchanged.

This release is the next bounded step of the six-part strategy, not all six completed: independent-provider failover, automatic quarantine/readmission, session affinity and passive native attribution remain future gated engineering. No accounts/proxies are deleted or globally disabled; retry/deadline/ownership/recovery policy is unchanged.

Source acceptance and pending release evidence: RESIDENTIAL_CATEGORY_EXPANSION_20261009.json. Production activation requires all full/vet/race/source-review gates, clean build, backup, scoped native canary, then fresh fleet verification. No replacement exclusion pilot is started.
