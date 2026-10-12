## 2026-10-12 — Linode expiry deficit recovery verified

- Product source `c839c0c4daac4d230b1ffabd78972cf9be80a128`; public main/stable and both management runtimes `5b0dc7640eaf51909ae04a1710aae15bf34088ab`. Existing accounts, proxies, profiles, credentials, port/path and resource identities preserved; both hosts3units active and5readiness checks pass. Both GitHub Actions runs succeeded.
- Old Linode107935963 expired09:12UTC and was blocked belowDesired10. New Linode-only path requires fresh native inventory, exact local ownership, enabled valid backfill route and a same-account verified serving sibling; no active/unknown operations or unresolved refill debt. Durable claim and separately revalidated start preserve cancellation, crash and identity fences.
- Native lifecycle DELETE for107935963 succeeded with one attempt at15:59:05UTC. Fresh provider inventory confirms absence. Exact linked backfill `107979802` completed READY/PANEL_COMPLETE, routingAPPLIED, fresh5/5target users; recovered_at receipt persisted. Existing account Output contains the replacement in both Direct and Residential classes with no foreign/expired-resource entries or class overlap.
- The new record closes backfill only after actual linked serving readiness and retains that result through later replacement expiry. No manual provider DELETE, quota guessing or scheduler/proxy bypass. Desired10 and normal20–30minute growth settings preserved.
- Validation:58unchanged packages passed full isolated PostgreSQL/opt-in browser suite. One old minimal fixture lacked accounts.provider; amended it and reran the complete app package under race, plus final targeted expiry/identity/backfill races, successfully. Vet and independent source review PASS. Initial failed receipts preserved. Public source export/scanners/unprivileged fulltests and exact-sourceUbuntu24installupdate passed before atomic push.
- OpenAI API for this correction:3responses,59738input+3673output=63411tokens. Previous video phase168914; known combined232325tokens. Billed dollars unavailable, not inferred.
- Exact Linode quota remains unknown; current count is not the configured target10. Other provider lifecycle policies unchanged. Growth continues normally.
- Evidence `/root/backups/dob-expiry-deficit-20261012`, per-host `/root/backups/expiry-deficit-deployment-20261012`, publisher `/var/lib/datacenter-publisher/release-t8in__ia`. This entry supersedes the preceding unresolved expiry note. Canonical detached-head workflow and its existing bootstrap warning remain unchanged.

## 2026-10-12 — Video repair verified and deployed

- Product checkpoint `8626a1ade83c38262636163f64a786961f0079a4`; public main/stable and both management runtimes `7829f9d05dc4e543fbc4594f0556533d00142cf3`. User explicitly authorized fixes, publication and rollout. Both GitHub Actions runs succeeded.
- Fixed Linode duplicate wire labels using immutable account/deployment identity; preserved ownership tags, reconciliation, durable operation keys, quota and spacing policies. Native scheduled Linode `107975848` completed create, SSH/bootstrap, panel installation, verified routing and inventory. Its configured client target was fulfilled. Growth toward desired10 continues at20–30minute spacing; exact provider quota remains unknown.
- Remaining lifecycle observation: old Linode107935963 is still EXPIRING under the established deficit-backfill policy (managed3 < desired10); no provider deletion has been attempted. Do not claim deletion acceptance. Keep this visible in the next operational check.
- Build Activity uses mounted keyed polling, preserves scroll and modal/navigation state, retains last-good data, and shows authoritative global active/waiting counts. Terminal/deleted history is excluded. Dashboard separates total tracked servers from health categories and exposes bounded actual reasons.
- Existing account share links returned2DIRECT and6RESIDENTIAL configs across the two new Linodes; no foreign or expired server URI and no class overlap.
- Historical provisioning errors aggregate without deployment join fanout. Expected precheck exit1 no longer duplicates command-stage failure; real faults remain observable. Native execution confirmed PRECHECK_MISS followed by successful bootstrap without false STAGE_FAILED.
- Full isolated PostgreSQL and opt-in browser regression:59Go packages PASS; vet, provider/executor/workflow race, read-model race, mobile/desktop build and config/dashboard read-recovery tests PASS. Independent source review PASS after narrowly correcting observation suppression. Initial failed reports/test fixtures retained as evidence, not erased.
- Public pipeline exported immutable allowlisted source, passed Gitleaks and live-secret scans, unprivileged regression and exact-source Ubuntu24LXC update. Both management updates preserved existing account/proxy/profile/server identity and login settings;5readiness checks and actual running binary hashes passed. No public export of private history/evidence.
- OpenAI API for this task:8responses;151371input+17543output=168914tokens. No billed USD inferred.
- Evidence `/root/backups/dob-video-audit-20261012`, `/root/backups/video-audit-deployment-20261012` on management hosts, publisher `/var/lib/datacenter-publisher/release-m4k8scr_`. Full video mapping and receipt summaries in `docs/development-jobs/video-audit-20261012.json`.
- Canonical remains detached by established workflow. Existing branch-name bootstrap warning is preserved, not claimed passed. This entry supersedes older runtime/version boundaries below.

# Project State — Digital-ocean-bot
Updated: 2026-10-01
Repository: /root/projects/Digital-ocean-bot-canonical-e2e
Remote: git@github.com:rezajafari0970/Digital-ocean-bot.git
Branch: checkpoint/final-e2e-20260929
HEAD: 6834d813fc16a6b4aefd05f2cb8696c18b0c0fec

## Runtime
- digital-ocean-bot-api: active
- digital-ocean-bot-worker: active
- Production runtime: /opt/digital-ocean-bot

## Current Vultr state
- Interactive Vultr console / reverse-proxy / noVNC work is deployed.
- Short-lived authenticated HttpOnly browser session exists for console tabs.
- noVNC websocket proxy path was corrected.
- Root noVNC websocket alias is accepted by commit 6834d81.
- Recent chain: 7b79dad -> 7c9f335 -> af598d7 -> 7126567 -> 6834d81.

## Current verification target
Refresh Admin, open Vultr Console, and verify the fresh ticket/session reaches interactive noVNC. Cloudflare/CAPTCHA/2FA remains a manual browser step when presented.
