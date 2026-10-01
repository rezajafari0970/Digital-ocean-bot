# Deep Requirements & Intent Brain

Commit `9a57a8921e235351c57943af23724dc7e119474f` — **101 requirements across 18 knowledge areas**.

This expands the coarse requirement brain into durable behavioral intent. It exists to teach a fresh session *why* the project behaves as it does, not merely where code lives.

## architecture
- `DEEP-ARCHITECTURE-01` — provider-neutral core
- `DEEP-ARCHITECTURE-02` — provider-specific behavior remains in drivers
- `DEEP-ARCHITECTURE-03` — modular independently testable subsystems
- `DEEP-ARCHITECTURE-04` — source/runtime are authoritative over stale prose

## accounts
- `DEEP-ACCOUNTS-01` — account CRUD and provider identity
- `DEEP-ACCOUNTS-02` — independent account isolation cell
- `DEEP-ACCOUNTS-03` — explicit account/provider states
- `DEEP-ACCOUNTS-04` — optional browser credentials do not replace API token flow
- `DEEP-ACCOUNTS-05` — automation settings remain account-scoped

## digitalocean
- `DEEP-DIGITALOCEAN-01` — preserve mature DigitalOcean behavior
- `DEEP-DIGITALOCEAN-02` — provider catalog drives region/size/image availability
- `DEEP-DIGITALOCEAN-03` — classified provider errors
- `DEEP-DIGITALOCEAN-04` — capacity/history/snapshots remain observable
- `DEEP-DIGITALOCEAN-05` — catalog synchronization does not regress active lifecycle

## vultr
- `DEEP-VULTR-01` — provider-specific API semantics
- `DEEP-VULTR-02` — accurate capacity is first-class
- `DEEP-VULTR-03` — interactive browser capacity observation isolated from generic provider core
- `DEEP-VULTR-04` — manual challenge path remains human-interactive
- `DEEP-VULTR-05` — authorized browser state may be reused after manual challenge

## proxy_network
- `DEEP-PROXY-NETWORK-01` — HTTP/SOCKS proxy model
- `DEEP-PROXY-NETWORK-02` — per-account proxy binding
- `DEEP-PROXY-NETWORK-03` — sticky identity while healthy
- `DEEP-PROXY-NETWORK-04` — country affinity and controlled fallback
- `DEEP-PROXY-NETWORK-05` — required proxy failure is fail-closed
- `DEEP-PROXY-NETWORK-06` — network identity/geo context remain account-scoped
- `DEEP-PROXY-NETWORK-07` — health/recovery must not silently leak direct egress

## lifecycle
- `DEEP-LIFECYCLE-01` — desired server count is reconciled state
- `DEEP-LIFECYCLE-02` — build spacing and concurrency are controlled
- `DEEP-LIFECYCLE-03` — one coherent lifecycle execution owner
- `DEEP-LIFECYCLE-04` — creation/readiness/install/panel stages are observable
- `DEEP-LIFECYCLE-05` — expiry/deletion/replacement are reconciled
- `DEEP-LIFECYCLE-06` — recovery and retries remain idempotent where possible

## provisioning
- `DEEP-PROVISIONING-01` — provision run/steps are observable
- `DEEP-PROVISIONING-02` — SSH readiness/host-key behavior is explicit
- `DEEP-PROVISIONING-03` — installer registry and selection are modular
- `DEEP-PROVISIONING-04` — retry/backoff and error classification are visible
- `DEEP-PROVISIONING-05` — rearm/recovery does not duplicate ownership
- `DEEP-PROVISIONING-06` — post-install completion is separate from provider create success

## sanaei_panel
- `DEEP-SANAEI-PANEL-01` — panel abstraction remains modular
- `DEEP-SANAEI-PANEL-02` — Sanaei API is live read/write plane where implemented
- `DEEP-SANAEI-PANEL-03` — authentication/session behavior is resilient
- `DEEP-SANAEI-PANEL-04` — inbound/client mutation and inventory are API-driven
- `DEEP-SANAEI-PANEL-05` — desired/create/update/planner/ready-worker responsibilities remain separated
- `DEEP-SANAEI-PANEL-06` — panel bootstrap and user capacity are observable

## reality
- `DEEP-REALITY-01` — Reality credentials are generated/managed explicitly
- `DEEP-REALITY-02` — target discovery/observations/scoring/selection are dedicated concerns
- `DEEP-REALITY-03` — SNI selection follows policy/target finder rather than arbitrary fixed target
- `DEEP-REALITY-04` — probe/health verification is explicit
- `DEEP-REALITY-05` — Reality contract/global policy remain separate from provider lifecycle

## config_users
- `DEEP-CONFIG-USERS-01` — global config and port policies are configurable
- `DEEP-CONFIG-USERS-02` — inbound allocation follows policy
- `DEEP-CONFIG-USERS-03` — bulk user creation scales without changing semantics
- `DEEP-CONFIG-USERS-04` — quota/lifetime/device limit remain independent controls
- `DEEP-CONFIG-USERS-05` — slot/capacity handling reflects live panel state
- `DEEP-CONFIG-USERS-06` — client deletion frees capacity

## output
- `DEEP-OUTPUT-01` — Output uses Sanaei/API state rather than SSH scraping
- `DEEP-OUTPUT-02` — Output is fast/live
- `DEEP-OUTPUT-03` — snapshots support performance without stale validity
- `DEEP-OUTPUT-04` — direct and share views use same canonical visibility rules
- `DEEP-OUTPUT-05` — first_seen/last_seen are preserved
- `DEEP-OUTPUT-06` — visible_until/exact expiry controls disappearance
- `DEEP-OUTPUT-07` — configs may disappear shortly before scheduled invalidation
- `DEEP-OUTPUT-08` — refresh must avoid OOM regressions
- `DEEP-OUTPUT-09` — no-cache user expectation is preserved

## residential
- `DEEP-RESIDENTIAL-01` — residential CRUD/test is independent
- `DEEP-RESIDENTIAL-02` — panel association/outbound tag/priority are explicit
- `DEEP-RESIDENTIAL-03` — Sanaei synchronization is isolated from account control-plane proxy
- `DEEP-RESIDENTIAL-04` — residential country/egress behavior is observable

## database
- `DEEP-DATABASE-01` — migrations are append-only ordered history
- `DEEP-DATABASE-02` — migration checksum integrity is enforced
- `DEEP-DATABASE-03` — live schema must be checked before schema-sensitive work
- `DEEP-DATABASE-04` — continuity snapshots store metadata not user rows/secrets
- `DEEP-DATABASE-05` — schema ownership is traceable to code and migrations

## admin_api
- `DEEP-ADMIN-API-01` — routes remain explicit and traceable to handlers
- `DEEP-ADMIN-API-02` — auth protects mutating/admin operations
- `DEEP-ADMIN-API-03` — account/provider/config/output/proxy/residential APIs preserve module boundaries
- `DEEP-ADMIN-API-04` — errors should remain classifiable/actionable
- `DEEP-ADMIN-API-05` — settings changes do not silently alter unrelated modules

## frontend
- `DEEP-FRONTEND-01` — seven operational module pages remain navigable
- `DEEP-FRONTEND-02` — UI actions map to explicit APIs
- `DEEP-FRONTEND-03` — mobile/operational usability is preserved
- `DEEP-FRONTEND-04` — refresh/save should preserve working context where possible
- `DEEP-FRONTEND-05` — interactive challenge flows provide visible browser feedback
- `DEEP-FRONTEND-06` — frontend must not invent backend state

## tests
- `DEEP-TESTS-01` — focused subsystem tests precede broad verification
- `DEEP-TESTS-02` — provider regressions have characterization tests
- `DEEP-TESTS-03` — runtime-visible changes require real flow smoke verification
- `DEEP-TESTS-04` — test maps trace features to verification commands
- `DEEP-TESTS-05` — handoff knowledge gates do not substitute for product tests

## deployment_runtime
- `DEEP-DEPLOYMENT-RUNTIME-01` — builds are stamped with commit/time
- `DEEP-DEPLOYMENT-RUNTIME-02` — artifact hashes bind binaries to build manifest
- `DEEP-DEPLOYMENT-RUNTIME-03` — health and readiness are checked after deploy
- `DEEP-DEPLOYMENT-RUNTIME-04` — production revision claims require verifier success
- `DEEP-DEPLOYMENT-RUNTIME-05` — Git HEAD may differ from production until deploy
- `DEEP-DEPLOYMENT-RUNTIME-06` — secrets never enter continuity docs

## continuity
- `DEEP-CONTINUITY-01` — new chat starts from canonical repo not main assumptions
- `DEEP-CONTINUITY-02` — knowledge artifacts are commit-pinned
- `DEEP-CONTINUITY-03` — exact line questions use source snapshot not guesses
- `DEEP-CONTINUITY-04` — intent/history/impact are read before behavioral changes
- `DEEP-CONTINUITY-05` — audit evidence is preserved
- `DEEP-CONTINUITY-06` — 100/100 means tested recoverability/traceability not unaided token memorization

