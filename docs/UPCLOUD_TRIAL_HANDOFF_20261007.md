# UpCloud trial-compatible mode — 2026-10-07
Status: TRIAL_TWO_SERVERS_READY_RESIDENTIAL_UNAVAILABLE. Trial deployment and native panel/listener proof passed; full client traffic and residential egress are not claimed.

User explicitly authorized adapting the deployment to the existing free trial. Known account Upcloud 113, 63cf71fa-ad4b-4ee0-a72b-b3e3cad22fbe, was blocked by TRIAL_FIREWALL version6. Ordinary account refresh did not resolve it.

Migration160 adds default-off per-account opt-in. The authenticated API and authorized local operator CLI share one admission-locked transaction. It requires enabled/non-deleting UpCloud, no in-flight create, mode not previously enabled, allowed configured client ports and the exact current TRIAL_FIREWALL version. Mode authorization and historical denial release are audited atomically; it never asserts trial removal. A new provider denial re-blocks, including after opt-in. No synthetic application administrators or sessions are created.

New deployment snapshots pin the account's mode, overriding untrusted profile input. UpCloud requests firewall on without firewall-rule changes and keeps explicit IPv4 interfaces. Panel config uses3389 from the immutable snapshot and its persisted ledger; client ports80/443 are the only eligible trial client ports. Existing deployments/other providers retain their settings.

All12 current residential endpoints use prohibited outbound ports3106..4630 or10000. Trial-pinned panels exclude these endpoints from residential routing. Protected traffic remains fail-closed; no direct fallback or relay around the provider restriction. UI clearly reports restricted mode and zero compatible proxy endpoints. No residential success is claimed. Trial time/quota/egress constraints remain provider-controlled.

Tests: full Go, isolated PostgreSQL race/fault including stale/duplicate/version/auth/audit rollback/new-denial/snapshot pinning/panel3389/proxy exclusion, JavaScript syntax. Initial fixture failures retained in evidence. OpenAI review found rollback ignored pre-droplet trial snapshots; fixed and regression-tested. Final review PASS required by checkpoint script.

Rollback: before opt-in, old binaries remain compatible with additive migration160. After opt-in, pause affected trial accounts and settle/clean their trial-pinned deployments before an older worker can be restored. Do not erase actual provider-denial evidence. Down migration refuses opted-in accounts or any active trial-pinned deployment, including queued builds without droplet allocation, and live trial droplets.

Evidence: /root/backups/dob-upcloud-trial-20261007.

## Live result and continuation
- Trial source1384d16 was deployed and opt-in applied with the shared root operator CLI to exact TRIAL_FIREWALL version6; no synthetic administrators/sessions.
- Two current servers are READY/PANEL_COMPLETE: 212.147.228.165 (panel e8b83162-6edf-419d-8807-9dc69ba9778a) and 83.136.250.142 (panel e83bed21-1660-4ab0-a843-914d90f35ef6). Both panel3389 and active VLESS/Reality443; TCP443 reachable. Native Sanaei acceptance passed34 route probes for the first current panel. Replacement route tests await an observed DIRECT client (only RESIDENTIAL present); its native settings/rule checks passed before that prerequisite. No policy/client manipulation was performed to manufacture evidence. This proves routing decisions/settings, not phone/VLESS end-to-end traffic.
- Fresh provider reads through the account proxy confirm both started, firewall on and IPv4 only. Provider capacity2 is full; capacity-related PROVIDER_BLOCKED now means no additional slots, not a renewed TRIAL_FIREWALL denial. No active account_create_blocks row.
- All12 configured residential endpoints remain incompatible with allowed Trial outbound ports. Protected Ads/BrowserLeaks paths stay blocked. To enable those paths, supply a genuinely supported endpoint on an allowed port or change provider/account constraints; never rename ports or silently fall back to direct.
- First server0055baa4 failed bootstrap because UpCloud's upcloud_cmdline.cfg overwrote99-dob-kho.cfg. Automatic failure cleanup removed it and created the second current ready server. Failure evidence remains; no attempt reset, snapshot rewrite, manual boot repair or fabricated success.
- Kernel ordering fix fc971058a555718cdd178cd226b038c65b79383f retains known99 and adds exact owned idempotentzz drop-in. New readiness requires both. It passed actual shell-glob simulation, interrupted journal, missing/unknown/symlink artifact, duplicate/conflicting option and boot-argument preservation tests. OpenAI reviewPASS and full Go passed. Live repair on the affected deleted kernel was not run/claimed.
- Current API/worker both fc97105, schema160; static1384d16; guardian source remains8e75d91. Clean frozen worktree and /proc executable hashes match. Unrelated account/proxy/route/profile settings preserved.
- Current two droplets retain normal automatic lifetime/rotation (at observation expiry22:48:19Z and22:53:46Z on Oct6). Do not confuse ordinary rotation with permanent free hosting.
- Next useful user test is a real client connection to443. Full residential Ads traffic remains unavailable with current endpoints. Native upstream x-ui expiry restarts and original Vultr232 billing-error classification are separate unresolved work.
