# Phase 3: isolated worker progress supervision
Status: candidate. Runtime remains50f9ffb until deployment acceptance is recorded.

## Contract
The independent internal/supervision package tracks module loop progress, declared idle intervals, admission waiting, and separately timed work/stages. A heartbeat cannot refresh an unrelated loop or task deadline. Work accounting starts after shared admission; queued children suspend the ancestor budget while occupying no execution slot. Native workflow and SSH script phase deadlines remain authoritative. Explicit nested phase budgets pause only their ancestors; siblings remain independently watched.

On a latched progress fault or lost SQL role ownership, Modules cancels the role and joins for at most15 seconds, then returns a fatal process error. It never launches replacement goroutines or frees an unfinished handler's admission slot. The next process uses the existing durable checkpoint/ownership/unknown-outcome reconciliation. Normal termination joins up to75 seconds; systemd kills the control group at90 seconds. Startup has a3-minute context and3-minute15-second hard bound after restart admission.

## External supervision and durable pacing
Both split units use Type=notify, main-process notification, WatchdogSec=45, WatchdogSignal=SIGKILL, RestartSec=15, TimeoutStartSec=15min. READY requires every registered module's first real progress state, a successful registry check and no queued failure. WATCHDOG is emitted after a successful independent supervision check, without SQL.

Per-role local restart ledgers are exclusive for the process lifetime, bounded in size, atomically replaced and fsynced. Cooldown grows to10minutes, survives restart and wall-clock changes, and resets only after5minutes of supervised healthy operation. Corrupt state refuses admission without overwriting evidence. The15-minute systemd startup allowance covers the capped cooldown plus bounded initialization.

## Isolation and preserved policy
Control/panels retain separate OS processes,24/40 SQL budgets,3/10 shared work admission, independent512MiB memory limits and SQL role ownership. API pool24, schema162, static assets and all operator settings remain unchanged. The host, PostgreSQL and network are still shared failure domains. Stalled work restarts its role; unrelated role survives. This is progress supervision, not proof that external services succeed.

Provider restrictions, finite retry budgets, quarantine, uncertain remote outcomes, ownership and disabled operator gates remain authoritative. Recovery does not override them. No production fault injection and no claim that every future failure can be prevented.

## Verification
tools/phase3-supervision-acceptance.sh retains phase1/2 native HTTP/PostgreSQL uncertainty and ownership fixtures, failure ledger gates, race tests, pool contention, full Go suite and systemd syntax validation. New tests exercise hung work despite fresh loop pulses, noncooperative cancellation, stage/sibling/idle/admission timing, initial readiness, clock and ledger corruption, long downtime, actual SQL cancellation and role backend loss during durable work. Real workflow and SSH runner wiring use an injected clock to exercise legitimate long phases without waiting15minutes.

The process fixture starts an empty isolated PostgreSQL schema, private state directory, random master key and actual compiled worker under transient notify units. It SIGSTOPs panels, uses an accelerated5-second external watchdog, and checks replacement PID, fresh supervision, persistent restart count, unchanged control PID and duplicate-role refusal. The same fixture is repeated against the frozen committed artifact before rollout. Fixture units have network access restricted to loopback; production services are untouched.

## Rollout and rollback
Install deploy/worker-control.conf at the existing control60-worker-isolation.conf drop-in and the panels unit at its normal systemd path. Require effective control/panels ExecStart, Type=notify,45-second watchdog, fresh heartbeats from BOTH current PIDs and all five readiness checks. Compare running binary hashes and account/profile/gate/proxy/routing/protection snapshots. Preserve original binaries and systemd files.

Rollback stops BOTH producers, restores checkpoint-aware50f9ffb binaries with their matching simple-type units, reloads systemd and starts the old roles. Schema162 and restart ledgers remain. Never pair old binaries with notify units; never roll back to checkpoint-unaware code over unresolved rows.

Evidence: /root/backups/dob-phase3-supervision-20261007. See paired acceptance/source snapshot for executed results, review IDs, exact hashes and bounded native output observations.
