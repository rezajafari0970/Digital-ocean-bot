# Production recovery and isolated controller — 2026-10-23

Operational recovery and software remediation are deployed. Cloudflare DNS capacity and physical attribution remain explicit external limitations. Read this entry before historical same-day acceptance notes. Never restart the quarantined primary database or workers.

## Released and running

Public stable/main: `17a1ce99f3e52a9151baaa0f9228b73ec381decf`, exported from private product source `889070fcbee68a212e246a487afeabbf588c6003`. Both independent secondary instances run that exact release with migration195 and verified API/worker hashes. All five readiness checks pass, all six service restart counters are zero after their planned starts, and all14 preservation comparisons pass. Existing secondary account count remains zero.

Public checks:144 mandatory database/race regressions with zero skips; full Go/vet/Python/Node/shell checks; exact LXC install, ordinary login,15 assets and five readiness checks. GitHub main run40003899321 and stable40003899945 both succeeded. Live secret scans compared31021 private values without logging values. Evidence: primary `/var/lib/datacenter-publisher/release-ibylizqj`.

## Incident and faithful recovery

The 20:31 Tehran video showed database recovery required, unavailable account/build pages, stale provider errors and incomplete TLS publication. All64 half-second frames were inspected;32.257seconds of audio yielded no intelligible transcription.

A new catalog-index checksum fault started at16:41UTC after the earlier KHO-off reboot. The primary cluster was cleanly stopped and preserved in place and in `/root/backups/dob-video-2031-20261023/offline-original`. No checksum bypass, counter reset, global cache flush, original-page replacement, row removal or historical full-database rollback was used.

Offline checking found13 bad pages. Buffered versus aligned direct reads covered2010 nonempty relation files /2185461760bytes and found41 differing pages. Some direct variants passed where buffered variants failed;32 other direct variants failed while buffered variants passed. Both variants and headers/LSNs were preserved. A separate candidate assembled checksum-valid variants and passed full offline checksums (2408files/266780blocks), strict pg_amcheck, logical dump and full dump parsing.

The independent secondary restore matched all167 table counts/content digests, covering2415797rows before workers resumed. This proves faithful migration from the validated candidate, not historical semantic correctness of every source value. Guest kernel evidence did not establish the physical cause. The primary remains unsuitable for production database/worker use.

## Isolated topology and controls

- Public URL remains `https://85.192.63.181:8443/admin/`; primary nginx uses a pinned-key, restricted SSH forwarding service to secondary loopback28080.
- Secondary production PostgreSQL18/production uses55482, system identifier7699915772003124533, databaseOID16385. Existing18/main5432 and its application remain independent.
- Production runtime/configuration/state/health: `/opt/dob-production`, `/etc/dob-production`, `/var/lib/dob-production`, `/var/lib/dob-production-health`.
- Units: dob-production-api, worker, worker-panels, integrity, ingress and independent updater. Actual namespace inode/device bindings were checked. The startup guard checks both privileged cluster identity and the application's own DSN/database identity, retained master key and durability settings; incomplete migrations are fenced.
- Only internal panel port changed18080→28080. Persistent IPv4/IPv6 nftables rules block non-loopback28080. Original accounts, policies, credentials, output settings and selective Google Ads settings are preserved.
- Primary original PG/app/publisher/backup/integrity services are quarantined by a marker and unit conditions; original PG start.conf is manual. An actual start attempt left all four PG/app units inactive with ConditionResult=no and PID0.
- Killing the forwarding process deliberately caused automatic replacement and restored public readiness in4.11seconds.
- The production API sees its own update socket. An actual API restart changed only the production PID; all default-instance PIDs stayed unchanged. Both a verified no-op and a complete release update succeeded through the fixed-action socket.
- Root-owned adapters and unit definitions are versioned under `ops/production-instance`. They fetch public stable, reuse shared staging/toolchain helpers, preserve data transactionally, verify exact running artifacts and roll back automatically. Four adapter failure/success tests pass. HTTP requests cannot choose commands, paths or URLs.
- The root CLI `dob-production recover` handles an interrupted deployment under the same locks. Production integrity/backup support is pinned inside its own runtime, independent of default-installer revision cleanup.

## Corrected defects and acceptance

1. Account summaries lacked an unconditional latest-snapshot index. The canonical-only partial index missed legacy/null snapshots. Actual SQL fell from13390.788ms/233770readblocks to20.997ms/45blocks. Migration195 adds the correct index; the regression uses2500 historical rows plus a newer legacy snapshot and proves a bounded lookup. Normal API samples were0.083–0.138seconds; a concurrent-build/backup sample was2.282seconds.
2. A real populated upgrade exposed install-bootstrap's two-minute deadline. The transaction rolled back at schema194 and the old runtime was restored. The bounded deadline is now15minutes, with all retention checks intact. The successful real migration continued beyond the old limit and retained all data.
3. Latest installed revision now comes from the current manifest even when a CLI update supersedes an older panel job. Eleven updater tests include this regression and isolated target selection.
4. Long health messages and proxy/account headings wrap correctly. Eight pages at320/360/390CSSpixels passed24 checks comparing scrollWidth to clientWidth, with no JavaScript exceptions. This avoids the old false pass caused by a mobile layout viewport expanding to342px. The displayed long warning was injected only in the diagnostic browser; real readiness remained ready.
5. Twelve public API checks returned200. Account, dashboard, Build Activity, Residential, Configs, Output, Additional IPs and Proxies rendered. AWS form secret masking, session-token input and validation-before-save were checked without submitting credentials.
6. The earlier AWS DescribeRouteTables fix (100-item limit and explicit operation pagination rules) remains included and regression-tested. There is no saved AWS account, so live credential validation or EC2 creation is not claimed.

Final production preservation at18:43UTC covered4556 common profile identities:4532unchanged,24 independently confirmed expired retirements,zero unexplained changes,zero missing identities. All77 Trojan identities remained identical. Normal new server/listener creation continued.

## Backups

Production integrity runs on a persistent six-hour UTC calendar with a finite next deadline. Inspection is followed by a fresh sealed logical dump and configuration, then encrypted restic archival. The independent adapter reuses only a dump completed within five minutes; it does not reuse the default project's20-hour daily window. Transient deployment markers are excluded. Failed integrity/backup cannot replace a good backup.

Three verified local dumps are retained. The dedicated encrypted repository keeps the last three and three daily representatives grouped by host/tags. The primary hourly forced-read-only SSH pull stages a complete encrypted generation, reads every pack, then promotes it while retaining the previous generation. Current original incident evidence remains separately preserved. Backup restore proof includes manifest/hash checks, master-key identity and full pg_restore parsing.

## Remaining limitations

Cloudflare has one enabled zone with a reported200-record quota; one observation was195/200. Six other zones are disabled and were not silently enabled. Current policy creates2records/server every1800seconds, with2700–3600second lifetimes and max6active/server. At86eligible servers, two overlapping generations can exceed available zone capacity. This causes real DNS-waiting TLS listeners; it is not database corruption.

At18:43UTC, the three non-CDN profiles had86/86 verified and85ready (one pending). TLS had69/86 verified,68ready,17blocked; all12 residential proxies were healthy. Counts change with normal expiry, DNS rotation and observation. No complete TLS coverage claim is made. The offered policy choice was not answered; the existing two-record policy remains unchanged. Resolution requires fewer records per batch, additional explicitly selected enabled-zone capacity, or a higher provider quota. Do not erase pending/error states to make the dashboard green.

Physical cause on the old host remains unproven. Keeping the same public IP also retains that host as the forwarding gateway; it no longer owns production data or worker execution. No absolute no-recurrence guarantee is justified.

## Controlled OpenAI API usage

CapUSD0.50 and3requests; request cap reached, no further paid calls.

| Purpose | Model | Tokens | Estimated USD |
|---|---|---:|---:|
| Recovery review | gpt-6.1-sol |3269|0.027682|
| Migration review | gpt-6.1-sol |3218|0.027868|
| Video audio,32.257seconds | gpt-4o-mini-transcribe |322|0.00161284695|
| Total | |6809|0.05716284695|

These are estimates, not an invoice. The two reviews informed preservation, read-variant selection, independent restoration and split-brain prevention. No automatic request retries; credentials remained server-side. Ledger:`/root/backups/dob-video-2031-20261023/api-usage.json`.

## Evidence and continuation

Primary:`/root/backups/dob-video-2031-20261023`; secondary:`/root/dob-production-recovery-20261023`. Important evidence includes preservation-proof.json, final-preservation.log, default-final-verification.log, live-verification.json, failure-isolation-proof.json, updater-isolation-proof.json, sealed backups and the encrypted off-host restore proof. Diagnostic admin sessions were removed.

After cleanup, temporary broad migration access and the publisher DB tunnel must remain removed/stopped. The restricted production gateway and backup keys remain. Do not re-enable the old primary automatic publisher or original workers. Follow the capacity choice above for TLS, and investigate/replace the primary host before using it for data again.

## Final cleanup and restore acceptance

At18:52UTC the fresh dump20261023T184952.377876Z (490141786bytes) was restored from off-host encrypted snapshot09a04bdd63e7aea487c76b14d70099a51d0f1864560302c4c3604ccdf4182d5b. Sealed file hashes, full dump parsing, both master-key copies and operational source inclusion passed; transient deployment markers were absent. Secondary checksum counter remained zero.

At18:53UTC the exact temporary broad migration key was removed from secondary authorized_keys; permanent restricted gateway/backup keys were retained. The temporary publisher DB tunnel was stopped, temporary scanner configuration/key copies removed, old evidence timer disabled and disposable dobtest24 stopped. All original corruption evidence and quarantined database files remain preserved.
