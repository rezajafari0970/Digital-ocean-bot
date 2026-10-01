# ChatGPT Master Context

## Mission
Build and operate one modular automation platform for cloud accounts, compute lifecycle, proxy/network identity, Sanaei/x-ui deployment, Reality configuration, live config Output, and provider-specific capacity discovery. DigitalOcean is the mature baseline; Vultr is being integrated without weakening provider isolation.

## Source of truth
Canonical source: `/root/projects/Digital-ocean-bot-canonical-e2e` on `serverprojects.ptr.network`. Never assume GitHub main is production truth. On every new session read this file plus `PROJECT_CONTEXT.md`, `PROJECT_STATE.md`, `HANDOFF.md`, then verify HEAD/status/services before edits.

## Current scale
The Go codebase currently contains about 408 Go files, 121 test files and 102 up migrations. Web UI is `web/static/{index.html,app.js,app.css}`. Runtime entrypoints live under `cmd/`; API and worker are the primary services.

## Architectural spine
`cmd/api` wires the Admin/API surface. `cmd/worker` runs asynchronous lifecycle work. `internal/app` is orchestration/composition. `internal/providers` is the cloud-provider abstraction and registry. Provider implementations are isolated under `digitalocean`, `vultr`, and browser-console support under `vultrconsole`. `internal/droplets` owns compute lifecycle/reconciliation. `internal/network` owns proxy/egress isolation and health. `internal/accounts` owns account context/cells/identity. `internal/panels` owns panel lifecycle and Sanaei integration. `internal/adminapi` exposes management APIs consumed by the web UI. PostgreSQL schema evolution is in `migrations/`.

## Provider model
DigitalOcean implementation includes account discovery, catalog, pagination, resources, mutations, error classification and DriverV2. Vultr has separate client/read/mutation drivers and types. Vultr console/browser automation is deliberately separate from API provider logic and exists to support interactive browser-only flows and capacity discovery when authoritative API information is insufficient.

## Panel/Sanaei model
Panel logic is modular: create/update orchestration, desired state, inventory, listeners, policy allocation, Reality contract/scan/config builder, health verification, user capacity, residential sync and Sanaei driver/runtime/session/API modules. Live Output should be derived from Sanaei/API state, not SSH scraping. SSH remains an installation/session capability where required, not the Output data plane.

## Product behavior
Accounts and proxies are independent modules. Account/provider state must be explicit and observable. Proxy assignment supports account isolation/sticky behavior and health handling. Server creation is scheduler/lifecycle driven, capacity-aware, and reconciled rather than treated as a one-shot script. Panel installation/configuration follows compute readiness. Reality/VLESS is the primary current config path. Output is intended to update rapidly and suppress expired/soon-invalid entries using persisted visibility/expiry semantics.

## Safety of project state
Never reset or delete uncommitted/audit material without inspection. Never put tokens, passwords, cookies, API keys, private keys or session secrets into continuity docs or Git. Preserve migrations and deployed-state compatibility. Before deploy: tests/build/migration compatibility. After deploy: service health plus functional smoke test. Every completed logical stage gets commit + push + continuity-doc update.

## Current Vultr workstream
Vultr capacity must be determined accurately, analogous in purpose to DigitalOcean capacity but using Vultr-specific authoritative behavior. Interactive Vultr console access was added through a reverse proxy/noVNC flow so a human can solve Cloudflare/CAPTCHA/2FA when required. Recent commits added capacity browser probe, challenge session, short browser session cookie, corrected noVNC websocket routing, and root websocket alias support. Current verification is a fresh Admin login/session -> Open Vultr Console -> fresh ticket -> successful WebSocket/noVNC connection -> manual challenge if shown -> reuse authorized state for capacity/account workflow.

## How the next ChatGPT session should behave
Do not restart design from zero. First establish current truth from Git and production. Read relevant code before proposing edits. Preserve existing modular architecture and naming. When the user says “next logical step,” continue from HANDOFF, test the current boundary, then implement the smallest complete production-grade step including tests, deploy/checkpoint where appropriate. When debugging, inspect actual logs/code/runtime rather than guessing. Keep the user informed of concrete results, not hypothetical progress.
