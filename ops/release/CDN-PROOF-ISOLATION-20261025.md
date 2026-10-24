# CDN proof isolation and AWS diagnosis — 2026-10-25

Status: released, deployed and verified on both independent instances. AWS credentials, DNS capacity and public gateway management remain external blockers.

## Exact release evidence

- Private tested source: `4acfdf0a0f4b9a1925e49ca144e1d3a7c61e7db7`.
- Public release: `a5f53f7dedd2bf22a12b423d85bbff0f9f011192`; previous stable `f6b2815ca0d6c85edde7753d6a9c7ca15e5ca31a`.
- [Exact-head full Go/vet, browser, database/native/race gate](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40140632279): 185 required regressions passed, zero skips.
- [Public source, restrictive Ubuntu installation and repeated installation](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40140892646): passed.
- [Atomic main/stable promotion](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40141113861): passed.
- Exact export manifest and gitleaks passed. Live-secret comparison passed against production (35,996 values) and the independent default installation (15 values), without logging values.

## Corrected behavior

A partial CDN probe previously overwrote the listener-wide class proof, including a class independently verified through another endpoint. Eligible sibling proofs now aggregate with their original timestamps. A newer DIRECT success cannot renew or revoke an older RESIDENTIAL success.

Negative authenticated traffic or public TLS evidence clears only the tested endpoint and current revision. Output and fallback aggregation share the same profile/account binding, enabled state, revision, operator, DNS readiness, observation age, expiry, zone and strict TLS requirements.

Structural native binary/socket/user failures continue to block the affected listener. Inconclusive observations cannot renew evidence. Cancellation during public probes is inconclusive. No publication freshness window or lifetime was relaxed.

AWS background errors now retain allowlisted wire code and operation, rather than only AUTHENTICATION_FAILED. Raw provider messages and unknown metadata remain redacted.

## Confirmed external blockers

At 2026-10-24T22:46:43Z, a scoped read-only health request through the existing AWS runtime and required proxy reached STS GetCallerIdentity and received HTTP 403 InvalidClientTokenId. This independently establishes rejected stored authentication; intermittent proxy errors do not negate it. Replace matching credentials through the panel, including session token for temporary credentials. Do not reset TOKEN_INVALID or bypass eligibility to claim recovery. The diagnostic dispatched no compute mutation.

Only one Cloudflare zone is enabled, with quota 200 and initial observed usage 194. Six other zones remain disabled. The active schedule requests two records every 1,800 seconds, lifetime 2,700–3,600 seconds, max active six per panel. Approximately 100 eligible panels imply mean steady demand of approximately 350 DNS records before reserve. This exceeds available capacity. Increasing enabled capacity or changing count/lifetime requires a concrete user choice; existing zones and schedule were preserved.

Public gateway port 8443 differs from internal listener 28080. Gateway management remains inaccessible, so public-port editing remains unfinished. Changing only the internal listener would break forwarding. The previous guarded redirects and root-owned public-port declaration are preserved.

## Resource handling and API accounting

Heavy tests ran only on GitHub-hosted runners. Live builds used MemoryMax=1G, MemoryHigh=768M, GOMEMLIMIT=384MiB, one Go build package, and CPUQuota=100%.

The first rollout stopped at the unchanged disk reserve guard before service replacement. The verified staged runtime was retained. Thirty-two closed journals older than 24 hours were archived losslessly: 813,694,976 source bytes into 66,806,542 bytes, with tar byte comparison before unlinking the originals. Reproducible cached test-source files were checked against Git blobs before removal; unused npm caches were also removed. Manual cache cleanup removed no database backup or unique source file. The installer retained its standard verified retention behavior, including expiry of old copies. Free space reached 6,658,076,672 bytes; the ordinary reserve and integrity guards were retained for resumption.

New external OpenAI API usage: **0 requests, 0 tokens, USD 0**. No accessible key is present on the healthy controller; the authorized key is on the offline legacy host. No Astra review was performed or claimed. Historical ledger: 3 requests, 6,809 tokens, estimated USD 0.05716284695. Project tools did not switch the chat model.

Private evidence is retained under /root/dob-cdn-continuation-20261025 on the healthy controller. The quarantined legacy host was not used for builds or database work. Customer credentials, raw configuration links and video frames were not sent to external review APIs.

## Final live verification

Both instances run the exact public release with matching executable hashes. All six services are active, each instance passed all five readiness checks and all fourteen preservation checks, and no post-update automatic restart was observed. Production preserved all 5,584 pre-existing config-profile ownership records and 77 Trojan ownership records unchanged. Credentials, environment, account rules, Cloudflare settings and user scheduling policies are unchanged. Schema remains 000195_provider_snapshot_latest.

After default-instance installation, production was verified again to establish isolation. Reproducible Go caches were cleared only after builds ended; final free disk space was 6,843,052,032 bytes. Three old clean Git worktrees were removed after proving their commits are ancestors of the released private source; all branches and Git objects remain available. The active worktree is /root/projects/dob-cdn-proof-isolation-20261025.

The public gateway served the expected assets over normal validated TLS. In the 23:15 UTC sample, OVH, Linode, Vultr and DigitalOcean sample accounts each had ten ready unexpired servers. OVH published 41 links in each class, Linode 43 Residential links. AWS still had one ready server, rejected account authentication, stale native proofs and zero sampled output. Proof admission requires an ACTIVE provider account; no eligibility guard was bypassed. Accounts without existing share capabilities were not counted as zero output.

At 23:16 UTC, the scoped CDN coverage sample contained 103 panels: 15 had no live DNS record, 20 had no fresh verified endpoint, 29 had one verified endpoint and 54 had at least two. These changing live counts do not establish zero-gap DNS rollover or solve the structural quota shortage. The endpoint-isolation regressions establish the code behavior; capacity and credentials still require action.

Gateway Desktop Commander remained offline (19 hours at the final device check). Resume public-port work only after access is restored, and keep the quarantined host out of application builds and database execution.
