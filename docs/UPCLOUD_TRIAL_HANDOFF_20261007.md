# UpCloud trial-compatible mode — 2026-10-07
Status: PREDEPLOY_TESTED_LIVE_PENDING. Do not claim live success yet.

User explicitly authorized adapting the deployment to the existing free trial. Known account Upcloud 113, 63cf71fa-ad4b-4ee0-a72b-b3e3cad22fbe, was blocked by TRIAL_FIREWALL version6. Ordinary account refresh did not resolve it.

Migration160 adds default-off per-account opt-in. The authenticated API and authorized local operator CLI share one admission-locked transaction. It requires enabled/non-deleting UpCloud, no in-flight create, mode not previously enabled, allowed configured client ports and the exact current TRIAL_FIREWALL version. Mode authorization and historical denial release are audited atomically; it never asserts trial removal. A new provider denial re-blocks, including after opt-in. No synthetic application administrators or sessions are created.

New deployment snapshots pin the account's mode, overriding untrusted profile input. UpCloud requests firewall on without firewall-rule changes and keeps explicit IPv4 interfaces. Panel config uses3389 from the immutable snapshot and its persisted ledger; client ports80/443 are the only eligible trial client ports. Existing deployments/other providers retain their settings.

All12 current residential endpoints use prohibited outbound ports3106..4630 or10000. Trial-pinned panels exclude these endpoints from residential routing. Protected traffic remains fail-closed; no direct fallback or relay around the provider restriction. UI clearly reports restricted mode and zero compatible proxy endpoints. No residential success is claimed. Trial time/quota/egress constraints remain provider-controlled.

Tests: full Go, isolated PostgreSQL race/fault including stale/duplicate/version/auth/audit rollback/new-denial/snapshot pinning/panel3389/proxy exclusion, JavaScript syntax. Initial fixture failures retained in evidence. OpenAI review found rollback ignored pre-droplet trial snapshots; fixed and regression-tested. Final review PASS required by checkpoint script.

Rollback: before opt-in, old binaries remain compatible with additive migration160. After opt-in, pause affected trial accounts and settle/clean their trial-pinned deployments before an older worker can be restored. Do not erase actual provider-denial evidence. Down migration refuses opted-in accounts or any active trial-pinned deployment, including queued builds without droplet allocation, and live trial droplets.

Evidence: /root/backups/dob-upcloud-trial-20261007.
