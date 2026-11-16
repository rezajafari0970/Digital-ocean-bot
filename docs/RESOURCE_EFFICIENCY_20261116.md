# Resource efficiency — accepted 2026-11-16

Both Digital-ocean-bot installations run `4c4be6978301f6e7d1c263250f7d58e59828b8c0` with schema 207 unchanged. Source `a0c81e89a9ecdba87c23d42aef15c1e0214cb640` merged through [PR 50](https://github.com/rezajafari0970/Digital-ocean-bot/pull/50) as `b8110c157f6150e125e3cbda3eae531adbae6c60`. Public main/stable were atomically promoted to the exact tested candidate. This documentation-only checkpoint does not require another runtime deployment.

## Measured result

Three approximately 20-second samples before and three after, covering the six application services and their two dedicated PostgreSQL clusters on the same two-core host. Normal output remained 40 rows from 20 panels in all six samples. The PostgreSQL processes were not restarted. These short observations are not peak-load or long-term availability guarantees.

| Metric | Before | After | Interpretation |
|---|---:|---:|---|
| Project plus databases CPU, percent of one core | 93.76% | 64.77% | 30.92% lower; approximately 46.88% to 32.39% of this two-core host |
| Six application cgroups, MiB including cache | 101.23 | 100.03 | Small change; no major application RAM reduction claimed |
| Project plus database cgroups, MiB including cache | 752.56 | 1224.73 | Increased after verified backup; not reported as RAM savings |
| Default database transactions per second | 185.20 | 91.80 | About half the idle polling transactions |
| Production database new sessions per second | 12.80 | 11.06 | Reduced churn; remaining sessions retained for existing work |

Detailed RAM inspection found 52.30 MiB of anonymous memory across the six application services. Production PostgreSQL had about 688 MiB of file cache, mostly inactive, after the integrity backup. The host still reported 2.20 GiB available; recent per-project memory pressure was zero and no cgroup OOM event was observed. No global cache flush, database restart, smaller database buffer allocation or artificial reclaim was used to improve the displayed number. Existing per-worker 256 MiB soft / 512 MiB hard protection remains. Total RAM reduction is not established by this run.

## Runtime change

Independent process-local discovery gates coalesce empty profile-queue scans across sibling consumers. A durable SQL claim opens the existing parallel capacity before slow remote work begins. Create, delete, routine verification and recovery verification have separate gates. Stale empty results cannot override newer claims; cancellation, panic cleanup and late callbacks are covered. Existing one-second wakeup, worker counts, work budgets, SQL ownership, expiry, routing, collection cadence and failure records remain authoritative. No SQL pool/advisory safety guard was weakened and no feature was disabled.

## Disk and permanent maintenance

Fifteen inactive production runtime copies, totaling 1,012,032,786 logical bytes, were converted into verified archives totaling 451,567,870 bytes. Archive savings: 560,464,916 bytes. The separate unused, reproducible project build cache reclaimed 369,528,832 bytes. Combined attributable savings: **929,993,748 bytes, about 930 MB**. This is not a net filesystem delta: new checkout/build/backup files and normal activity also consume disk. Database history and active data were not removed or rewritten for space.

`deploy/rollback-archive.py` is a separate maintenance driver. It accepts only exact dated production runtime directories, retains three newest and every copy younger than seven days, checks current binary manifests and all running process references, and shares installer locks. It verifies archive member set, content, permissions and identity before replacing the inactive directory with a durable archive pointer. Interrupted removal resumes only after revalidating the archive and remaining source; corruption or uncertainty preserves data. All historical content remains recoverable in archives.

`dob-resource-maintenance.timer` is enabled and active across reboot. It runs hourly with jitter, at most two old runtimes per run, Nice 19 / idle IO / CPU weight 10, 25% of one CPU and 128 MiB memory. It does not impose that CPU cap on real provider work. Status and failures are recorded separately. Current binaries, database, settings, credentials, backups, unknown cloud-create evidence and unrelated project trees are excluded.

## Verification

- [Source fleet run 43784122244](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/43784122244): **401 required test roots passed, zero skipped**. Full Go/vet, browser/native, disposable database/race and restore/interruption gates passed.
- Five archive safety/recovery tests and focused Go race tests passed.
- [Public candidate run 43784452875](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/43784452875): source checks plus fresh Ubuntu installation and repeated-install identity preservation passed.
- [Atomic promotion run 43785402676](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/43785402676) passed. Both bounded live updates completed with verified native backups.
- 21 preservation groups per installation passed; all 1903 production profile identities and 9 Trojan identities stayed unchanged. All six running binary hashes, ten readiness checks, HTML and 32 assets per installation matched; public gateway TLS verification passed.
- Six other project services retained identical PIDs, start timestamps, active state and restart counts. No other project source/settings or shared global resource policy was changed.
- The initial copied asset-check fixture still expected the old revision. It stopped acceptance after the first update. All 32 hashes plus HTML were independently matched to the tested candidate; only the expected revision was corrected. The first failure receipt was retained and the check passed before production deployment.

## Existing provider failure and limits

Vultr had `PROXY_ERROR` **before** deployment, with diagnostic `IDENTITY_UNVERIFIED` and historical HTTP 401 authentication/source-IP-allowlist evidence. Its last successful provider observation was November 9. This resource change does not repair credentials or claim that account is healthy. Before deployment, the live gate was explicitly adapted to baseline nonregression: healthy-baseline accounts need a fresh successful observation; the already-failed account needs a fresh attempt with the same failure or a successful recovery; all require fresh healthy proxy routes and unchanged country policy. The original all-healthy gate and baseline are retained. Two Linode observations succeeded freshly; Vultr remains blocked rather than falling back to direct traffic. No new cloud resource was created or deleted for testing. No universal provider/browser/CAPTCHA or 99% IP-availability claim is made.

OpenAI gpt-6-astra review was attempted once with bounded input/output and a one-dollar/two-request session ceiling using the previously authorized durable ledger. The provider returned `credit_balance_exhausted`, no review and no usage receipt. There was no retry generation. The 0.236559 USD reservation is not an asserted charge; prior uncertain ledger entries remain intact.

Private detailed evidence: `/root/dob-resource-audit-20261116`. Read the adjacent JSON receipt for measurements and final checks. Preserve original-host quarantine and unknown-create receipts. No deployment remains pending. Further memory reductions need measured workload evidence; do not trade away useful cache or safety fences to manufacture a lower counter.
