# Post-release continuation — 2026-10-25

Status: observed recovery completed for all three tracked new servers. No product-code change or deployment was made in this continuation. Live release remains a5f53f7dedd2bf22a12b423d85bbff0f9f011192; prior checkpoint was f6262d28bb51193fef239681ed674e9a3a622e92.

## Current health

At 06:17 Tehran, all six services were active with zero automatic restarts since their previous deployment. Each independent installation passed all five readiness checks. Both version endpoints reported the expected unmodified release. Free disk space was 7,368,056,832 bytes. The public gateway subsequently served the expected assets over certificate-validated TLS.

The previous 185-test release gate remains the source-validation evidence. These are additional live observations, not a new test run or a claim that every future failure is prevented.

## Tracked automatic builds

Times below are Tehran on 2026-10-25. Durations are deployment creation to READY, including provider, bootstrap, required reboot, installer and post-install work.

| Provider | Created | READY | Duration | Result |
| --- | --- | --- | --- | --- |
| Vultr | 03:30:43 | 03:39:14 | 8m 31s | PANEL_COMPLETE / READY |
| DigitalOcean | 03:33:35 | 03:41:45 | 8m 10s | PANEL_COMPLETE / READY |
| Linode | 03:33:43 | 03:40:45 | 7m 01s | PANEL_COMPLETE / READY |

The existing installer scheduled required reboots for Vultr at 03:35:20 and DigitalOcean at 03:38:35. DigitalOcean readiness recorded transient SSH connection refusal/timeouts, then a successful readiness observation and completed installation. Linode progressed independently. No manual status change, duplicate install, forced retry, gate bypass or server creation was dispatched during this observation.

## Remaining blockers and input needed

- AWS: one read-only GetCallerIdentity check through the existing required proxy at 03:37 returned HTTP 403 InvalidClientTokenId. At 06:17, stored credentials were still unchanged and the account remained TOKEN_INVALID. Correct the matching credentials through the panel; temporary credentials also require their matching session token. The sample still had one ready AWS server and zero Residential output.
- DNS: only the existing zone is enabled; the six other account zones remain disabled. At 06:17, authoritative enabled-zone usage was 197/200. The scoped coverage sample had 103 panels, 15 without live DNS and 18 without fresh verified endpoints. Existing schedule demand remains above capacity. No new domain or reduction in record count was selected, so DNS settings were preserved. Choose dedicated additional domains/capacity or a revised production policy before changing this behavior; do not change another domain's TLS settings implicitly.
- Public port: gateway management remained offline, last seen about 22 hours earlier. Public 8443 continues to differ from internal 28080. Gateway access must be restored before public-port editing can be implemented and verified end to end.
- OpenAI API: the configured key files are absent on the reachable controller; the authorized legacy-host key is inaccessible. Astra was not called and the chat model was not switched.

At 06:17, sample OVH, Linode, Vultr and DigitalOcean accounts each had ten ready unexpired servers. OVH exposed 45 links in each class and Linode 42 Residential links. These are timestamped samples, not stable minimum output counts. Accounts without an existing share capability were not treated as zero output.

## Consumption and continuity

New OpenAI API usage in this continuation: **0 requests, 0 tokens, USD 0**. AWS provider health checks: **1 read-only request**; no compute mutation. Historical OpenAI ledger remains 3 requests, 6,809 tokens, approximately USD 0.05716.

Sanitized evidence accompanies this report. Detailed private observations are retained at /root/dob-postrelease-observation-20261025T0005 on the healthy controller. The active source worktree remains /root/projects/dob-cdn-proof-isolation-20261025. Keep the legacy host quarantined from application builds and database execution even when its management connection returns.
