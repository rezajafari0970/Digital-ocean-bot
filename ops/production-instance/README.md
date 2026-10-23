# Isolated production operations

These private, fixed-target adapters operate the migrated production instance on the secondary server. They are not part of the public installer. No credentials or keys are stored here.

- `instance-update.py` installs as `/usr/local/sbin/dob-production`: fetch approved public stable, build with the shared pinned toolchain and staging helpers, check independent identity and fresh sealed backup, quiesce only this instance, run retained migrations, verify exact binaries, automatically restore the prior runtime on failure. `recover` resolves an interrupted deployment under the same locks.
- `update-bridge.py` uses the reviewed public `deploy/project-update.py` module from the pinned application runtime; only fixed status/start/restart actions are accepted. Its socket is bound into the production API namespace.
- `verify.py`, `fresh-backup.py`, `archive-backup.py`, `update-bridge.py` install below `/usr/local/libexec/dob-production`. The runtime owns a pinned `support` directory containing public bootstrap, integrity, backup and compatibility helpers.
- `offhost-backup.py` installs on the primary as `/usr/local/libexec/dob-production-offhost.py`; it pulls the encrypted dedicated repository through a forced read-only SSH command. A complete new generation is checked before promotion, retaining the previous generation.
- `systemd` contains the deployed units/drop-ins. Gateway/offhost units belong to primary; other dob-production units belong to secondary. Review target identity and supply secrets through the existing root-owned configuration. Never apply these units to a different host by guessing paths.

Run `python3 instance-update-test.py` for rollback, failed rollback, successful deployment and corrupt-database rejection tests. Unit tests use only temporary paths and mocked services; real production acceptance is recorded in `docs/production-recovery-20261023.md`.

Backups retain three sealed local dumps. The dedicated encrypted repository keeps the last three snapshots and three daily representatives, grouped by host/tags despite changing dump directory names. Original incident evidence is retained separately.

The isolated integrity service runs inspection, then a fresh sealed backup (at most five minutes old), then encrypted archival. This deliberately differs from the default project’s daily dump reuse window. It excludes transient deployment markers and refuses stale/failed integrity inspection.
