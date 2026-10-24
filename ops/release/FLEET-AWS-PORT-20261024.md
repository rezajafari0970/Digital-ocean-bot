# Fleet, AWS and panel ingress repair — 2026-10-24

Status: released and deployed to both independent instances. Public gateway port editing and Astra API review remain blocked by inaccessible legacy management/credentials.

## Exact release evidence

- Private tested source: `4a74ac95c98714fbe5ed9f42360f3386a39407d6`.
- Public release: `f6b2815ca0d6c85edde7753d6a9c7ca15e5ca31a`; prior stable `6336c9e47f2907adc029024ea05fa433346b7e62`.
- [Independent full Go/vet, mobile/browser, native/race/ownership gate](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40088571639): 182 required regressions passed, zero skips.
- [Exact public source, Ubuntu 24.04 installation and repeated installation](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40089161394): both jobs passed.
- [Atomic main/stable promotion with leases](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40089388480): passed.
- Exact export manifest, live-secret scans against both instances and gitleaks passed. Neither user video nor credential-bearing configurations were sent to external review APIs.

## Changes and stages

| Stage | Result |
| --- | --- |
| Video/source/live investigation | Reviewed the full 297-second recording at one frame per second, including account forms, fleet state, build details and output counts. |
| Account editor / Save | Fixed undefined `d` references, restored modal ownership/submit handlers, kept optional selections empty, deduplicated payloads and verified Save/Enter across six providers and four mobile/desktop viewport sizes. Panel Settings submits independently after closing an account form. |
| Independent recovery | Separate two-job recovery class; weighted shared admission caps five jobs, 17 retained SQL cost and two expensive jobs while preserving provider/identity concurrency. Unchanged 24-connection control pool retains completion/ownership headroom. |
| Fleet counts | New explicit unverified count excludes Provisioning; legacy inactive API count remains compatible. |
| Output / Cloudflare | Prefer uncovered servers when allocating scarce DNS capacity. Fresh authoritative exhaustion receives a distinct diagnostic. No valid DNS record is deleted to increase capacity and no proof is bypassed. |
| Public versus internal port | Root-owned gateway declaration exposes public 8443 separately from internal 28080; redirects preserve public ingress and unsafe internal-port edits are rejected. Actual gateway port editing remains unavailable until gateway management is restored. Direct installations retain editable listen ports. |
| Verification and release | All required gates passed on exact revisions, both instances backed up and updated, both five-check readiness suites passed and all executable hashes matched. |
| Live recovery | Both tracked Vultr installers reached READY/PANEL_COMPLETE at 11:28:55 and 11:29:11 UTC. The tracked DigitalOcean SSH failure cleared and that server reached READY/PANEL_COMPLETE at 11:33:39 UTC. Additional newly scheduled replacements are distinct from these three recovered jobs. |

## Live preservation and output findings

At 11:31:55–11:31:57 UTC, all six instance services were active with matching executable hashes and zero post-deployment restarts. Each instance passed all 14 preservation checks. Production retained 5,156 pre-existing profile ownership records and 77 Trojan ownership records with zero unexpected changes; 12 legitimate expired listener retirements were independently identified, and 16 new profile listeners were added by normal work. Credentials, master key, account rules, user build spacing, creation/protection/output policies and Cloudflare settings were unchanged.

The public gateway served the exact new app.js and panel-settings.js over normal certificate-validated TLS. Its process has DOB_PUBLIC_PANEL_PORT=8443; the database listener remains 28080. Default-instance installation was also followed by a new production verification to establish isolation.

Three examples of observed output behavior:
- The AWS account has one live server and desired count ten. Its user-selected spacing is 15–260 minutes, with the next build recorded at 13:52:47 UTC (17:22:47 Tehran). This delay is a policy, not evidence that ten servers were already running.
- The only enabled Cloudflare zone has a quota of 200. Usage was 199 during investigation and 194 in the 11:33:58 UTC sample. Other zones remain intentionally disabled. A fully verified Cloudflare profile can publish multiple endpoints for one client, so eight confirmed class clients per server is not a fixed eight-URI output count.
- A ten-server OVH account produced 46 links in each class; a ten-server Linode account produced 44 residential links in the last sample. AWS residential output was observed as 4, then 3, then 4. Its old record crossed the five-minute expiry exclusion while replacements were activated; new endpoint traffic verification initially failed and subsequently recovered. This is not evidence of a permanent fixed output count or zero-gap rollover. Do not publish expired or unverified endpoints to inflate counts.

Snapshots without live share capabilities were not treated as zero output. The scoped output probe reused existing tokens privately and logged aggregate counts/protocols only.

## Resource isolation and rollback

During the earlier local canonical gate, the 4 GiB host experienced global OOM at 10:48:32 UTC (an unrelated supervisor Python process was killed). Both production worker role checks timed out and exited safely; systemd recovered them at 10:51:14 UTC. This occurred before the new release. MemoryMax=2 GiB did not sufficiently isolate the test from live load.

Final heavy tests ran on independent GitHub-hosted runners. Ordinary rollout builds used MemoryMax=1G, MemoryHigh=768M, GOMEMLIMIT=384MiB and one build package at a time. Observed production/default build peaks were approximately 737/768 MiB. No additional production worker restarts occurred during those bounded builds or after deployment.

Reclaimed approximately 2.52 GB of reproducible source/build/browser caches after checking inactive build processes and exact clean clone identities. No database backup was removed by that cache cleanup. Normal installer retention later removed its own expired verified runtime/backup copies. Fresh sealed backups and independent pre-update database links were retained. Disk reserve and ownership guards were not weakened. Schema remains 000195_provider_snapshot_latest.

Full private operational evidence is under /root/dob-fleet-aws-port-20261024 on the healthy controller. A sanitized aggregate accompanies this report. The quarantined legacy host was not used for builds or database execution.

## Remaining access-dependent work and API ledger

1. Public gateway port editing: the old gateway's Desktop Commander session remains offline and existing SSH authorization was denied. Changing only the application listener would break the tunnel. A verified gateway adapter/control path is still needed before this feature can be completed.
2. Requested Astra review: the authorized OpenAI credential remains on the inaccessible gateway and is absent on the reachable controller. No Astra request was made or claimed. The chat model cannot be switched through project tools.

| API scope | Requests | Tokens | Estimated USD |
| --- | ---: | ---: | ---: |
| This task | 0 | 0 | 0 |
| Historical ledger | 3 | 6,809 | 0.05716284695 |

The requested API target remains gpt-6-astra with max reasoning once credentials are available. Reuse a bounded input/output budget and record response usage before reporting consumption. Do not copy customer secrets, raw configuration links or video frames into API review input.
