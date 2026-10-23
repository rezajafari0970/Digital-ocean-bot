# Integrity scheduling follow-up — 2026-10-23

The primary controller's database timer was active/elapsed with NextElapseUSecMonotonic=infinity and no service execution this boot. A prior-boot persisted trigger stamp plus the old OnBootSec/OnUnitActiveSec policy reproduced the identical failure in a disposable real-systemd fixture. Secondary scheduling remained armed. Fresh manual standard inspection succeeded without a new checksum event; the acknowledged primary watermark remains one, secondary zero.

The integrity timer now uses a UTC calendar schedule every six hours, while retaining the 15-minute boot trigger, bounded jitter and Persistent catch-up. Calendar scheduling has a future deadline independent of a previous service activation. The explicit timezone keeps cadence stable across local timezone changes. No database repair, statistics reset, boot setting, account policy or cloud resource change is part of this follow-up.

Two real-systemd regressions cover a prior-boot stamp, finite next execution, missed-run catch-up, repeated execution and stop/start. The old timer fails the stale-stamp fixture. The corrected timer passes both. Normal release smoke runs these fixtures inside its isolated container and now checks that both installed health timers are enabled and actually waiting with a finite next execution, rather than checking active alone. Only the disposable fixtures accelerate time/jitter; production keeps six-hour cadence and five-minute jitter.

Final release 380209b1388ca19ecb92fc9fbfa5b202893764ec from product source dd1f8fc765e0b7f2ae470f4c858292131625f8c1 is installed and verified on both hosts. The normal publisher passed all 143 mandatory database/race regressions with zero skips, full Go/vet/Python/Node/shell checks, exact Ubuntu 24 LXC installation, and the enhanced real-systemd smoke gate. An explicit captured repeat of that exact smoke passed both timer tests and authenticated panel acceptance. Both GitHub CI runs succeeded: main 39970651276 and stable 39970651417. Evidence: /var/lib/datacenter-publisher/release-p9ib1l8k.

Both guest machines are KVM virtual machines with virtual block devices and no exposed physical ECC counters. No new kernel corruption/I/O/OOM event was found after the previous repair in the observed window. This cannot identify the underlying host RAM/storage fault. No saved AWS account exists on either host; actual credential onboarding remains pending through the panel.

Operational evidence: /root/backups/dob-integrity-timer-20261023. Direct OpenAI model API usage for this continuation is zero requests, zero tokens, USD zero.

Timer semantics: https://www.freedesktop.org/software/systemd/man/latest/systemd.timer.html

## Live acceptance

At 2026-10-23T18:07:29.373262+03:30, both installed integrity timers were enabled, active/waiting and had finite next calendar executions. Primary next execution: 2026-10-23 18:00:36 UTC; secondary: 18:04:30 UTC. Kernel-memory timers were also enabled and waiting, kho=off remained active and CMA counters stayed zero. No reboot was needed in this follow-up.

Both update commands exited zero. All six running API/worker executable hashes matched their manifests, three additional readiness observations per host passed, and failed units were empty. Installation performed fresh integrity inspections successfully; no new checksum event or reset occurred. The primary historical failure watermark stays at one (13:28:24 UTC), secondary zero. The repaired page and original corruption evidence were not modified by this continuation.

Master keys, environment, account credentials/settings, Cloudflare settings, output/creation/protection/profile policies and selective advertising settings matched the pre-update snapshots. All 4,484 common profile identities and all 77 Trojan identities were unchanged; eight newly created profile identities followed ordinary lifecycle activity. Secondary identity checks passed as well. Primary publication, host-evidence and host-backup timers were restored active; secondary does not host those publisher-specific services.

The 14:30:38 UTC fleet snapshot had 91 READY resources and two provisioning resources. All three original video runs remained COMPLETED. Three DigitalOcean accounts were at their provider capacity limits (3/3, 10/10, 10/10) with native warning/creation-blocked observations; those protections were preserved. These counts are time-specific observations, not a permanent fleet guarantee.

Remaining boundaries: zero stored AWS accounts on both hosts prevents actual credential onboarding acceptance; physical corruption attribution still needs host/hypervisor evidence. Do not mark either unresolved item as repaired. Direct OpenAI model API usage remains 0 requests / 0 tokens / USD 0, excluding ChatGPT/tool usage.
