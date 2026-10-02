# Account Build/Delete + Dataplane P0 Audit — 2026-10-02

## Build starvation root cause
Healthy accounts with desired capacity but zero managed servers had enabled schedules/profiles but deployment_profiles lacked a valid installer_ref. Scheduler correctly rejected them as automation_not_ready, while UI provider gating misleadingly reported Ready to build.

Permanent correction:
- scheduler auto-pins an enabled profile to the latest active installer that has xui_database+xui_panel capabilities when the existing installer_ref is missing/stale;
- account/profile edits preserve installer automation;
- scheduler claim latency reduced to five seconds.

Live evidence after correction:
- previously idle Vultr desired=5 reached managed=5;
- two previously idle DigitalOcean desired=1 accounts reached managed=1;
- all healthy enabled profiles report automation_ready.

## DigitalOcean permission/billing diagnosis
Historical generic Permission denied was emitted during replacement mutation before CREATE_DROPLET operation creation. DigitalOcean SSH-key API permission can differ from droplet permission.

Corrections:
- DigitalOcean deployment SSH identity falls back to cloud-init/root ssh_authorized_keys when provider SSH-key API returns permission denied;
- password SSH is disabled and root password is locked;
- provider billing/account restriction is classified separately as BILLING_BLOCKED / PROVIDER_BILLING_BLOCKED instead of generic permission denied;
- blocked accounts remain fail-closed and are reprobed so automation resumes when the provider restriction clears.

## Delete behavior
Delete operations are idempotent and recovery verifies provider existence before confirming local deletion. Current long-lived delete debt is limited to provider-blocked/locked DigitalOcean accounts; local state is intentionally not falsified as DELETED while provider resources may still exist.

## Reality/sniffing
Reality inbound enforces:
- sniffing enabled
- destOverride: http,tls,quic
- metadataOnly=false
- routeOnly=false
- tcpFastOpen=true
- tcpNoDelay=true
- tcpKeepAliveIdle=25
- tcpKeepAliveInterval=20
- tcpUserTimeout=30000

## Residential TCP/UDP/QUIC
Live residential proxy is healthy SOCKS5.
Corrections:
- SOCKS residential outbound sets settings.udp=true;
- residential routing rule uses network=tcp,udp for SOCKS5;
- HTTP residential remains TCP-only to avoid UDP blackholing;
- ordinary/direct traffic remains available independently;
- QUIC is carried as UDP when routed through the SOCKS5 residential outbound.

Residential reconciliation was changed from sequential shared-context fan-out to bounded parallel per-panel reconciliation (concurrency 12, per-panel timeout 5s), preventing one slow panel from cancelling the entire fleet.

## Host access hardening
New servers use per-deployment SSH keys, ssh_pwauth=false, and locked root passwords. A destructive wipe-on-any-login policy was rejected because legitimate automation uses SSH and such a trigger could erase healthy production hosts. The next security control should distinguish unauthorized/interactive sessions and respond with terminate + credential rotation/revocation + controlled replacement.
