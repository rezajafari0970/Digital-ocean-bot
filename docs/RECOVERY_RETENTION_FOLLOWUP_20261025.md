# Recovery rollout follow-up — 2026-10-25

Observed at 2026-10-25T19:57:06.853322+00:00. Both live instances run release `53294d3936a96fa342db4036fa87f910f289e1e9` with migration 197. Six services are active, have matching running executable hashes and zero automatic restarts since this rollout. Both five-check readiness responses passed; HTTPS certificate validation, frontend version and source asset hashes passed.

The completed rollout receipts confirm exit zero, 15 preservation checks per instance, preservation of all 6,183 baseline profile identities and 77 Trojan identities, and zero unexpected ownership/credential changes. Five independently confirmed expiry retirements and six normal additions were recorded at acceptance. Production's controlled API stop lasted 216.581 seconds; it is over.

At the live runtime sample (2026-10-25T19:55:47.253490+00:00), 14 successful scheduled retention cycles had removed 11,478 internal observation rows, with no recorded retention error. There were 159 eligible panel observations at that instant. Between the preceding sample and this one the eligible count fell from 654 to 159 across the next regular cycle. These are observation rows, not client configurations, and this is not a measured fleet deletion-latency claim.

Free filesystem space was 7,832,289,280 bytes. Current database checksum failures were zero; the latest integrity/backup receipt was healthy and both scheduled integrity timers were active. The journal pattern scan covers the new production processes from 18:44:41 UTC; it found zero panic, OOM, invalid-page/checksum, no-space, fatal or retention-error matches. It does not prove that all possible application errors are absent.

The canonical source branch was verified at `cbb49e4ef29ad4b60e35ab3052e7ab6f2fb95325` and public main/stable at the installed revision before this follow-up. Previously accepted source/native/race/browser and fresh/repeated installation gates remain the validation basis; this follow-up changes evidence and continuity documents only.

## Open items

- Three requested deletions remain on one AWS server. Live database status still says TOKEN_INVALID; the account status timestamps are preserved in the JSON. Previous TCP probes timed out for panel and SSH from both controllers. No absence or successful deletion is inferred from those failures. Restore valid authorized credentials via Accounts, or obtain independent provider-confirmed deletion evidence.
- The original cluster 18/main and old application/publisher services were freshly checked inactive; its HTTPS gateway remains active. Historical corruption is confirmed; its underlying storage cause is still unproven. No old cluster was restarted or repaired.
- Whole-project manual audit remains incomplete at 269 current files. Continue the existing ownership/resource module inventory. Android-device acceptance and measured configuration deletion latency remain open.
- No new paid OpenAI request was submitted. The previously unknown request and reserved USD 1.501011 remain unreconciled.

The local progress record previously contained stale `deployment=not_started` and a pending retention description despite completed receipts. Those fields were reconciled from successful receipts and fresh live checks; the prior record was preserved. This corrects continuation metadata, not production data.

Private receipts remain under `/root/dob-recovery-followup-20261025` on the recovery host. The companion JSON stores the live measurements without credentials.
