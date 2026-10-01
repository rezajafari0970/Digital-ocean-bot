# Historical Decision Ledger

Purpose: preserve project intent and lessons that cannot be reconstructed reliably from source code alone. Source/runtime still wins for exact implementation state.

## Architecture decisions
- Cloud providers are plugins/drivers behind a provider-neutral Registry/Factory and canonical models. Scheduler/Lifecycle/Core must not accumulate DigitalOcean/Vultr branches.
- DigitalOcean is the mature reference implementation, but Vultr is not a clone: provider-specific catalog, credential, pagination, capacity and error semantics stay inside its driver.
- Canonical lifecycle is `Account -> Proxy -> Provider -> Server -> Deployment -> Provisioning -> Sanaei -> Panel`.
- Proxy is an independent module. Account egress is isolated and fail-closed; shared proxy/network identity between accounts is a regression.
- Scheduler/UI enqueue work; execution must avoid competing deploy executors. Historical duplicate-installer failures showed that in-process locks are insufficient across processes. Long-running install work belongs to controlled worker execution with per-resource coordination and visible/streamed diagnostics.

## DigitalOcean baseline decisions
- DO must remain fully functional while provider-neutralization proceeds; new provider work must not regress it.
- Provider observations are intentionally focused on account/catalog/servers rather than accumulating unrelated browser state.
- Provider errors are classified rather than treated as generic failures; retired/unavailable images must be recognized as image availability failures rather than retried blindly.
- Desired defaults established during provider cleanup: 5 region choices, 3 plan choices, Ubuntu-only image choices (historically 22.04/24.04/26.04 where actually provider-available), and random server lifetime around 90–120 minutes. Availability from the provider remains authoritative.
- Optional account email/password browser credentials must not replace API-token behavior. When absent, token-only behavior remains unchanged.

## Vultr integration decisions
- Vultr must use the same provider-neutral core but its own authoritative read/mutation/capacity behavior.
- Known integration traps that must not return: DO-only image validation in generic paths; unknown capacity blocking StartDeployment; UI hiding Vultr as development-only; reconciliation hard-coding DigitalOcean; artificial Vultr pagination ceilings; assuming Vultr credential lifetime/shape equals DO.
- Vultr reached an E2E server lifecycle through READY/PANEL_COMPLETE during integration; provisioning used public SSH-key/cloud-init readiness and Sanaei API for panel operations.
- Accurate Vultr account capacity is a first-class requirement. Where normal API data is insufficient, browser-console observation is isolated in `vultrconsole` rather than leaking browser logic into the provider-neutral core.
- Cloudflare/CAPTCHA/2FA is human-completed in an interactive console. The system may establish/reuse the resulting authorized session; it should not turn challenge handling into hidden core/provider logic.

## Proxy / identity decisions
- Each account must have an independently managed network identity; accidental shared DataImpulse/proxy state across accounts was identified as an isolation violation.
- Proxy type detection, health, sticky assignment and failure behavior belong to proxy/network modules, not cloud-provider drivers.
- A failed required proxy must fail closed rather than silently leaking direct provider traffic.
- Provider/account UI should expose actionable state classes rather than a generic red/green result where possible.

## Compute / provisioning decisions
- Server lifecycle is reconciled state, not a one-shot shell script. Creation, readiness, installation and panel completion have observable stages and retry/error classification.
- Historical architecture with multiple schedulers/workers/manual threads caused duplicate installs and stale-snapshot races. Do not reintroduce parallel owners of the same deployment state machine.
- Installation diagnostics must remain visible enough to identify SSH/script phase, retries and recurring sanitized error fingerprints.
- Provider resource creation and panel installation are separate boundaries: provider success does not imply panel readiness.

## Sanaei / Reality decisions
- Sanaei/x-ui is managed as a modular panel driver/runtime. Live panel/config reads should use Sanaei API where implemented; SSH is not the live Output data plane.
- Reality/VLESS is the current primary generated config path. Multiple inbounds/ports and SNI target selection are policy-driven rather than hard-coded into provider lifecycle.
- SNI/Reality target selection must use the dedicated target-finding/scan logic rather than arbitrary fixed targets when automatic selection is requested.
- User quota, lifetime, device limit and target users per inbound are policy controls and must remain independently configurable.

## Output decisions
- Output must feel live and fast. It must not become a slow SSH aggregation path.
- Snapshot persistence exists for performance/continuity, but stale visibility is not acceptable: expired or scheduled-invalid configs must disappear according to persisted visibility/expiry semantics.
- Earlier requirement established hiding configs shortly before scheduled deletion/expiry rather than waiting for a coarse estimated TTL. Do not regress to an approximate cache-only lifetime.
- Share links and direct Output views must apply the same visibility rules. Filters such as near-expiry/recently-created are views over the canonical valid set, not alternate stale stores.

## Residential routing decisions
- Residential egress is a separate panel-routing concern from account/provider proxy assignment.
- Residential routing was validated as working and is synchronized into Sanaei routing; keep country/egress behavior isolated from ordinary cloud account control-plane traffic.

## UI / operational decisions
- The Admin panel is mobile-friendly and operational: Accounts, Proxies, Residential, Configs, Output and settings are separate user-facing modules.
- Account edit/save/details and Output operations are expected to be fast and not jump/reset the user's working context unnecessarily.
- Remember-me behavior was intended to persist login for an extended period; frontend token storage and backend auth/session behavior must be considered together.
- Human challenge pages need an interactive browser path, not a fake success status or a background wait with no feedback.

## Development/process decisions
- Read the real source before making architectural changes. Audit work should be staged and evidence-preserving; do not mutate production during an audit-only phase.
- Canonical server repository/branch, GitHub checkpoint and production runtime must be distinguished explicitly.
- Never reset `.audit` material blindly. Never put credentials/session secrets in Git or continuity docs.
- Every completed logical stage should have focused tests, appropriate broader verification, commit/push, deploy verification when deployed, and an updated HANDOFF/PROJECT_STATE.

## Rejected/regressive patterns
- Provider conditionals scattered through Scheduler/Lifecycle/Core.
- Treating Vultr as a renamed DigitalOcean implementation.
- Shared account egress identity where isolation is required.
- Silent direct-network fallback after required proxy failure.
- Multiple independent executors owning the same install/deployment lifecycle.
- SSH scraping as the live Output source.
- Approximate Output TTL when exact client expiry/deletion timing is available.
- Hard-coded provider image assumptions in generic validation.
- Declaring a feature complete from source changes alone without production/runtime verification.
