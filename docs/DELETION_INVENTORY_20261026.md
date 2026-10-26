# Deletion inventory continuation — 26 October 2026

Recorded 2026-10-26T05:22:10.391593+03:30.

## Completed correction

Continued the exact pending candidate 4a5cd371 rather than repeating the already
accepted immediate-removal and port-reuse releases. Missing SSH key identities
could falsely confirm absence after a delete. A malformed later server entry
could also leave a durable adopted deletion target before the list was validated.
The account deletion path now validates the complete initial and post-delete
SSH inventories before recording progress, and validates the entire server list
before adoption. Vultr rejects a missing identity on every SSH inventory page.
Owned resources, unrelated accounts, retry evidence and the existing deadlines
are preserved. This change adds no provider requests.

## Stage results

| Stage | Verified result |
| --- | --- |
| Negative reproduction | Three regression roots failed as expected on the preceding code; run 40251016011 retained |
| Focused and full source validation | Runs 40252732989 / 40252755916 passed; 237 required roots, zero skips; 20 isolation repetitions |
| Other source gates | Full Go/vet, real browser, native/race and isolated database restore/interruption passed |
| Source integration | PR 17 merged as d3dfae92c0fd2cfbfd66d2feb19e26b34ba9f98a |
| Export protection | All 1,755 exported files passed both live-secret comparisons and gitleaks |
| Exact independent installation | Run 40254844466 passed initial and repeated installation |
| Atomic publication | Run 40255068645 passed; main/stable both 2755f995e78829669c22a671df4ad0b4db7e6a9c |
| Default installation | Update exit 0; 15 preservation checks, three executable hashes and five readiness checks passed |
| Production installation | Update exit 0; 15 preservation checks, three executable hashes and five readiness checks passed |
| Final live acceptance | Both instances on exact release; schema 198; HTTPS/TLS and served assets match; DB checksum failures zero |

Separate fresh baselines and readable custom-format database dumps were saved
before updates. Existing locks, integrity/backup guards, retention reserve and
atomic runtime replacement were retained. Builds ran under the established
1 GiB / one-CPU limit. The first production attempt stopped at insufficient_retention_workspace before
publication. Its logs and intent remain in production-rollout/attempt-1; the
previous runtime stayed healthy. After cache reclamation and a fresh state read,
the exact staged artifacts resumed through the same integrity, backup and reserve
guards. The original baseline was not replaced and no rebuild was needed.
These were controlled service restarts, not a claim of zero downtime.

One superseded routine pre-update dump (618,412,836 bytes) was copied to the
existing archive destination; content hashes and sizes were verified at both ends.
A durable archive pointer was written before releasing its local space. The fresh
backup and three incident/recovery archives were preserved. The original cluster
remains quarantined. After both builds completed and no compiler/package manager
was running, reproducible Go build cache and package indexes reclaimed about
457 MB of allocated space. All deployment reserve checks remained unchanged.

## Live deletion scope and remaining work

Active profiles at final observation: 2.
Profiles with detached cleanup still pending: 3.
Three unconfirmed listeners belong to an unreachable AWS server whose account
reports TOKEN_INVALID. Fresh SSH and panel connection attempts both timed out.
No remote-success receipt was fabricated, and no cleanup ownership was discarded.

Immediate removal from the active catalog/output and independent port reuse were
already deployed and remain in place. Two recent user-requested profile deletions
had native cleanup completion times of 52.49 and 56.10 seconds; these are database
observations, not UI-response latency or a fleet-wide speed guarantee. No new live
profile was created/deleted for this continuation's tests.

The comprehensive source audit and physical cause of the original storage failure
remain unresolved. No new paid OpenAI generation request was sent; the previous
unknown usage/reservation accounting remains unresolved. Do not reset that ledger
or restart the quarantined cluster.

Private receipts and per-stage state: /root/dob-continuation-20261026.
Machine-readable checkpoint: docs/DELETION_INVENTORY_20261026.json.
Next: recover authorized AWS access or verified provider termination to resolve
the three native removals; continue the existing module audit from its recorded
inventory. Do not repeat this accepted rollout.
