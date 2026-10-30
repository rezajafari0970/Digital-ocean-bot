# Storage cleanup — 2026-10-31 (Tehran)

The user explicitly authorized removal of unnecessary logs, hidden files, unused
large folders, scripts, old snapshots and backups while preserving this project
and the project listening on port 18443. That port was verified as the active
append-only encrypted backup-vault service, not an application directory to prune.

## Confirmed outcome

At 01:26:08 Tehran (2026-10-30 21:56:08 UTC), free space was 31,675,441,152 bytes,
up from 5,717,532,672 bytes at the initial check: 25,957,908,480 additional bytes.
Both application instances passed all ten readiness checks at the old runtime
67f91d2886e0b2b6dec6754700066a35eb2fccae. The vault remained active at the same PID
with zero restarts; its HTTPS endpoint returned the expected unauthenticated 401.
All eight collector processes remained running at their existing PIDs.

Completed removals:

- 60 closed old log files, old system journal segments and package download cache;
  this first phase recovered approximately 651 MB.
- 8,933 old downloaded attachments, predominantly APK/EXE files, totaling
  20,987,174,912 allocated bytes. None was open or referenced by a pending media
  job; the saved application policy already disabled retention of original files.
- 226,587 rejected temporary JSON files older than seven days, totaling
  935,739,392 allocated bytes.
- 77 legacy installation backups older than seven days and four old daily backups.
  Three current verified installation backups, three default daily backups and
  all three production daily backups were retained.
- The unused August installer staging tree. Its only scheduled-task reference was
  a commented provenance line. The three embedded links were unlinked without
  following them; runtime source, executables and Nginx configuration were retained.

## Final archive cleanup is not yet confirmed

A reviewed manifest selected 1,183,216 message-cache files older than seven days.
The collector's configured history window is 24 hours; the latest seven days,
active configuration outputs, queue jobs, account/session state and code remain.
The last readable progress receipt confirmed 659,559 of these files deleted.
Files are selected from exact directories and checked again for inode, size,
modification time and open descriptors before unlinking. No symlink is followed.

The already-started remote process runs independently of the connector. Desktop
Commander then reported its monthly usage limit and instructed no retry/reconnect.
Its final receipt and final configuration-hash verification could not be read.
Do not claim the last phase finished, and do not restart its deletion scripts.

Private evidence on the live host:

- `/root/dob-storage-cleanup-20261031/phase3-results.json`
- `/root/dob-storage-cleanup-20261031/phase3.log`
- `/root/dob-storage-cleanup-20261031/protected-file-hashes-before.json`
- `/root/dob-storage-cleanup-20261031/health-during-cleanup.json`
- The approved compressed deletion manifests and phase1/phase4 receipts.

The initial conservative staging deletion refused three symlinks before touching
that directory; a later explicit-link review allowed safe unlink-only removal.
Native retention pruned four daily backups; 77 unsealed legacy installation copies
were separately identified as expired after three newer sealed copies verified.
Both encrypted restic repositories, live databases, keys, current scripts, pending
release helpers and original-host incident recovery evidence were preserved.
No runtime update, database migration or application restart was performed.

## Resume

When remote access is available, read the existing final archive receipt first;
confirm `complete`, protected configuration hashes, free space and service health.
The last observed capacity exceeds the prior deployment reserve, but remeasure the
native reserve before deployment. Continue from the verified public release
1081be7edc22bb0cd50ccfc93852aa1650f73406 and its prepared bounded rollout helpers;
live runtime installation and provider-site acceptance are still pending.

The cleanup is one-time. Automatic expiry of these old attachment/message caches
was not added or changed. Application code/configuration was preserved.
