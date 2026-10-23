# Integrity scheduling follow-up — 2026-10-23

The primary controller's database timer was active/elapsed with NextElapseUSecMonotonic=infinity and no service execution this boot. A prior-boot persisted trigger stamp plus the old OnBootSec/OnUnitActiveSec policy reproduced the identical failure in a disposable real-systemd fixture. Secondary scheduling remained armed. Fresh manual standard inspection succeeded without a new checksum event; the acknowledged primary watermark remains one, secondary zero.

The integrity timer now uses a UTC calendar schedule every six hours, while retaining the 15-minute boot trigger, bounded jitter and Persistent catch-up. Calendar scheduling has a future deadline independent of a previous service activation. The explicit timezone keeps cadence stable across local timezone changes. No database repair, statistics reset, boot setting, account policy or cloud resource change is part of this follow-up.

Two real-systemd regressions cover a prior-boot stamp, finite next execution, missed-run catch-up, repeated execution and stop/start. The old timer fails the stale-stamp fixture. The corrected timer passes both. Normal release smoke runs these fixtures inside its isolated container and now checks that both installed health timers are enabled and actually waiting with a finite next execution, rather than checking active alone. Only the disposable fixtures accelerate time/jitter; production keeps six-hour cadence and five-minute jitter.

Local systemd verification passed. Normal full publication, exact installation and both-host deployment are pending at this checkpoint. Product source remains f548fb6 on both live hosts until that acceptance is complete.

Both guest machines are KVM virtual machines with virtual block devices and no exposed physical ECC counters. No new kernel corruption/I/O/OOM event was found after the previous repair in the observed window. This cannot identify the underlying host RAM/storage fault. No saved AWS account exists on either host; actual credential onboarding remains pending through the panel.

Operational evidence: /root/backups/dob-integrity-timer-20261023. Direct OpenAI model API usage for this continuation is zero requests, zero tokens, USD zero.

Timer semantics: https://www.freedesktop.org/software/systemd/man/latest/systemd.timer.html
