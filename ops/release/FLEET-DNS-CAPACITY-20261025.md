# Fleet DNS capacity release — 2026-10-25

Both independent installations now run `5dc2c25678698dbbe9ac1cfee7d18701c19213e7`. At 07:05 Tehran all six services were active, both five-check readiness suites passed, executable hashes matched, and reported automatic restart counters were zero.

Cloudflare status now estimates capacity across enabled profiles and account server targets, including replacement overlap. It subtracts unrelated DNS usage, respects selected domains and the existing 500-record worker ceiling per zone, and excludes domains without fresh access and Full (strict) verification. Stale or unknown quotas remain explicitly incomplete. The diagnostic is read-only, uses a three-second deadline and does not hide ordinary status or controls if it fails. The mobile card explains the shortage and keeps existing settings unchanged.

The observed plan covers 113 server slots, with 100 currently eligible servers. Its overlap allowance is 6 records per server: 600 for current eligible servers and 678 for the planned population. Known usable capacity was 185 records, a planned shortfall of 493. This is an advisory estimate, not a guarantee of verified links. Capacity alone does not establish credential validity, TLS correctness or traffic proof. No additional domain was enabled and no rotation/output-count policy was reduced.

Validation used exact private source `8c0e86b53fab8e14739f3680277f4c53b66a4ad2`:

- [Independent source, real mobile Chromium, database, native protocol and race checks](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40156990345): 187 required regressions passed with zero required skips.
- [Exact public source and independent installation/reinstallation](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40157154710): passed, including login, assets, readiness and identity preservation.
- [Atomic main/stable promotion](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40157364436): passed with the prior release pinned as the expected destination.
- Live-secret comparison against both instance configurations, gitleaks and exact export manifest verification passed.

Each installation passed all 14 preservation comparisons. Production retained 5,712 unchanged common profile identities; four other common identities were independently confirmed normal expired retirements, with no unexpected changes. All 77 common Trojan identities were unchanged. The public gateway served the exact new Cloudflare asset and cache-busted index over normally validated TLS.

Two automatically scheduled builds were observed completing without manual retries: DigitalOcean in approximately 8m43s and Vultr in approximately 5m11s. These observations do not promise a fixed provisioning time for every provider or machine.

Deployment initially stopped before runtime publication because the existing reserve requires three times database size plus 512 MiB. A first reviewed continuation also stopped at that guard. Four unused older runtime copies were archived losslessly and byte-compared; the two newest immediate rollback directories were retained. APT caches and exact reproducible exports were cleaned. No database backup was manually deleted; ordinary configured installer retention still ran. The previously verified stage was then deployed successfully, without rebuilding or weakening any guard. Final free space was about 6.82 GB. This remains a constrained host; check the reserve again before future updates.

A dedicated bounded launch was not evidenced for this turn's live builds: the observed default build belonged to the shared Desktop Commander service, so its shared peak cannot be reported as build memory usage. For future updates use `ops/release/run-bounded-update.sh`, which was checked against actual cgroup limits (1 GiB maximum, 768 MiB high watermark, one CPU) with a harmless process. It also supplies the existing conservative Go settings. This operator helper does not automatically wrap unrelated direct CLI invocations.

The remaining external blockers are explicit:

- A fresh read-only AWS STS check at 06:38 Tehran returned HTTP 403 `InvalidClientTokenId` for `GetCallerIdentity`. Correct matching credentials must be saved in the panel; temporary credentials also require their session token. Account eligibility was not bypassed.
- DNS needs additional usable quota/dedicated domains or an intentional operator change to the rotation policy. Six other known domains remain unselected. No choice was assumed.
- The gateway management device remains offline. Public port 8443 works and is identified separately from the internal listener 28080; editing the actual gateway port is unfinished.
- The requested OpenAI API key is absent from both known files and inline configuration on the reachable instances. Astra was not invoked. New external OpenAI API usage is 0 requests, 0 tokens and USD 0; chat billing is not exposed by these tools.

The legacy host remains quarantined for builds and database execution. Future work should start from this checkpoint, preserve the current release, and use fresh evidence for the external blockers.
