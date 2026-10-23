# Production recovery and isolated controller — 2026-10-23

IN PROGRESS. This supersedes the earlier same-day all-green timer acceptance. Do not restart the primary database or workers, or run the old automatic primary installer against production.

## Incident and evidence

The 20:31 Tehran video showed database recovery required, unavailable account/build pages, stale provider errors, and incomplete TLS profile publication. All 64 half-second video frames were inspected. At 16:41 UTC a fresh catalog-index checksum failure began; thousands of reads counted the same fault. The previous six-hour integrity timer was correctly scheduled; this was a new physical/read-path incident, not the old missing timer deadline.

Primary 85.192.63.181 production PostgreSQL18/main and all three application services were cleanly stopped. The original cluster is preserved in place and at `/root/backups/dob-video-2031-20261023/offline-original`. No reset of checksum statistics, checksum bypass, global cache flush, original-page write, row removal or full historical DB rollback was used.

Offline inspection found 13 bad checksums. Full buffered versus aligned O_DIRECT comparison covered 2,010 nonempty relation files / 2,185,461,760 bytes and found 41 differing pages. Some direct pages passed where buffered pages failed; 32 other direct pages failed while their buffered counterparts passed. Both raw variants and headers/LSNs were preserved. A separate candidate assembled only checksum-valid variants, passed complete offline checksums (2408 files / 266780 blocks), strict pg_amcheck, logical dump and full dump parsing. Passing checksums and identical LSNs are evidence, not proof of historical semantic correctness or physical cause.

The fault recurred after the earlier KHO-off reboot, with no identified guest kernel I/O/MCE report. Physical attribution remains unresolved. The primary is therefore quarantined as a database and worker host.

## Current topology

- Public address remains `https://85.192.63.181:8443/admin/` with existing nginx/TLS.
- Primary `dob-production-gateway.service` forwards only local 18080 through pinned-host-key SSH to secondary 176.124.198.111 loopback 28080. Its dedicated key cannot run shell commands and is restricted to that forwarding destination.
- Secondary independent PostgreSQL18/production, port55482, database digital_ocean_bot, system identifier7699915772003124533, database OID16385. Existing 18/main5432 and its separate application remain intact.
- Secondary pinned `/opt/dob-production`; configuration `/etc/dob-production`; runtime state `/var/lib/dob-production`; health `/var/lib/dob-production-health`; daily backups `/var/backups/dob-production-daily`.
- Units `dob-production-api`, `dob-production-worker`, `dob-production-worker-panels`, `dob-production-integrity.timer`. Required mount bindings were checked against actual running process inode/device identities. A root pre-start guard verifies exact database identity, retained master key digest and durability/checksum settings.
- API uses persisted `panel_settings.listen_port`, not HTTP_ADDR. Only the new instance internal port was changed18080→28080; external address remains8443. Existing schema intentionally fixes listen_host0.0.0.0. Dedicated persistent nftables ingress rejects non-loopback access to28080, including IPv6.
- Primary `/etc/dob-controller-quarantined` plus unit conditions fence original app/PG/publisher/integrity/host-backup services. Original PostgreSQL start.conf is manual. Primary publisher/backup/integrity timers disabled; the old publisher must not be re-enabled without adapting its deployment target. Disposable dobtest24 was confirmed to contain zero accounts and stopped.

## Independent preservation proof

Logical restore on secondary retained original ownership and master key. Restore errors were fatal. Strict amcheck and stopped-cluster full checksums passed; checksum counter zero. All 167 public-table row counts and order-independent row-content digests matched, covering 2,415,797 rows before workers restarted. This proves faithful migration from the verified candidate, not historical correctness of every source value. Source has 11 account rows (10 visible in normal account API), 4548 listener records,65408 snapshots and16026 Cloudflare records. Fresh integrity acknowledgement was issued only after these checks, followed by a verified new backup. No account credentials or encryption secrets enter this document.

## Independent performance defect

The account-list latest-snapshot subqueries lacked an unconditional `(account_id,created_at DESC)` index. The canonical-only partial index cannot serve latest legacy/null rows. Real EXPLAIN ANALYZE before correction:13,390.788ms,233,770 disk blocks read; afterward:20.997ms,45 blocks. Index was added concurrently to the live isolated production DB without modifying data. Migration195 makes this permanent for installation/upgrade, preserving the existing canonical-only index. Regression uses2500 historical snapshots plus a newer legacy snapshot, verifies correct latest value and no history traversal. Add it to the mandatory database/race gate.

Mobile page header/action/status wrapping is corrected without hiding errors. Actual mobile accounts, dashboard, Build Activity, Residential, Configs, Output, Additional IPs and Proxies rendered without JS errors or horizontal overflow. Final post-fix latency and profile convergence still need acceptance.

## API budget

Authorized OpenAI assistance used two bounded gpt-6.1-sol reviews and one gpt-4o-mini-transcribe request for the 32.257-second video audio. The transcription was empty. Total6809 tokens, estimatedUSD0.05716284695; capUSD0.50 and three requests. The request cap is reached; no further paid calls. Ledger and response usage under primary evidence root. Credentials remained server-side. No claim of exact billed cost.

## Evidence and remaining work

Primary evidence `/root/backups/dob-video-2031-20261023`; secondary `/root/dob-production-recovery-20261023`. Source worktree `/root/projects/dob-production-recovery-20261023`, branch `fix/production-recovery-20261023`; prior authoritative private head e9f13e1404c035a79efe01b47cd3471caf2b6f6d; pinned production public380209b1388ca19ecb92fc9fbfa5b202893764ec.

Remaining: finish new regression/full release/CI and deploy exact candidate to isolated production; verify repeat start/tunnel reconnection/retired-host fencing; complete independently recoverable off-host backup for new instance; inspect final TLS/DNS and residential/output coverage, not just page readiness; remove temporary broad migration key after copying necessary evidence; record final source/public revisions, tests and actual state. No saved AWS account exists; live AWS onboarding/create is not claimed verified. Existing provider capacities, normal expiry/deletion, upstream outages and in-progress DNS transitions must remain honest.

## Populated upgrade limit and instance control

Public51c9618 passed144 mandatory DB/race tests, full release gates, exact clean LXC installation and both branch CI runs. The existing secondary default project installed it successfully. The isolated production upgrade exposed a real scale limit: install-bootstrap canceled retained-row verification after two minutes on the 1.4GB / 2.4-million-row database. Its transaction rolled back, schema stayed194, and the old runtime380209b was restored; readiness and all three workers were verified with zero restarts. The concurrent account lookup index remains in place. No retention guard was disabled. The bounded bootstrap deadline is raised to15minutes, below the45-minute installer budget. Production-sized acceptance remains required.

The update bridge now exposes root-owned module target constants and a callable main entry point, preserving its strict socket action allowlist. A regression proves an independent adapter targets its own updater, lock and API restart service; requests cannot supply commands or paths. This supports isolation of the migrated production updater from the existing default project.
