# Account deletion reliability — 2026-10-06

## Incident and evidence
The user's 4.7-second recording shows Upcloud 11 already DELETE_PENDING, zero tracked servers, three worker checks and the old generic in-flight settling message. Read-only production inspection at 08:36 UTC found Upcloud 11 absent and no active account_deletion_jobs. It had completed before this patch; do not attribute that deletion to this release or claim an ongoing UpCloud provider outage. One older archived DigitalOcean account remains historical DELETED with no deletion job and is not reactivated.

## Shared fixes
- Explain safety settling, resource/operation/SSH-key work, next scheduled check and completion in Accounts. The required two-minute settling period is preserved, with retry at its boundary rather than an additional generic retry delay.
- Preserve the original deletion request time on retry. Repeated DELETE after removal returns ABSENT rather than misleading 404. Initial and progress counts use the same distinct resource inventory.
- Bound request lock contention and return retryable account_busy. Bound worker work internally to 45 seconds (earlier caller deadlines still win).
- Skip advisory-locked jobs using due-time keyset pagination; recheck due time after acquiring the lock. A locked page cannot exclude every later candidate.
- Retain account proxy maintenance while any durable deletion job exists, including the zero-server final inventory/SSH verification stage.
- Persist timeout/error status and retry together using a detached, bounded transaction; report persistence failures. The claim's durable retry remains if persistence is unavailable.
- Reconcile lost HTTP responses, suppress duplicate clicks and discard stale polling results. Preserve the surviving card's position, handle an empty list, and report removal. Local-only purge has a separate acknowledged action; it is never automatic.

## Safety boundaries
Deletion covers this application's owned/managed resources. The existing never-used-account fast path requires no droplets, resources, deployments or operations at all; it does not assert the entire provider account is empty. internal/droplets/executor.go persists Reserve/running before CreateServer, and admission/mutation fences prevent ordinary concurrent creation. Any history or unknown operation retains the provider verification path and credentials. Missing/corrupted historical journals require incident recovery rather than this fast-path contract.
Provider, credential, network or ambiguous-create failures remain pending and visible. An unknown create is not force-marked failed merely because a provider list is empty.
No production cloud account/resource was created or deleted for acceptance.

## Engineering and review
AI OS job: docs/development-jobs/account-deletion-20261006.json.
Plan response: resp_08b9687b9d197744006ac4b386551087d1af16aaa84641296f.
Source reviews: resp_0903a67d0ceb0953006ac4b5acd5b087d1a47ff58acf31fbb7 and resp_02f3f939db8a740c006ac4b6d8d4bc87d1a1b70f7c3878bf63.
Review dispositions:
1. Fixed locked-prefix starvation with keyset pagination and a 33-locked-account regression.
2. Fixed ignored partial status writes with an atomic transaction and propagated errors.
3. Confirmed the unchanged empty-never-used fast path using pre-network journaling and fences; final review accepts the stated ownership boundary.
4. Added an internal work deadline independently of the worker's existing 45-second caller deadline; fixture provider asserts a deadline even with a Background caller.
5. Replaced GREATEST inventory counts with distinct UNION counts; regression uses overlapping and disjoint resource IDs.

## Acceptance and rollout
Full Go suite plus focused race/PostgreSQL/browser acceptance. Fixtures cover all three providers, live resource still present, provider timeout, credential retention, original settle deadline, duplicate/lost response, rollback of failed requests, account isolation, non-admin rejection, lock contention, unknown-create visibility, 33 locked jobs, cleanup-network lifetime, mobile/desktop, reload, stale response and deleting the last account.
No schema migration. Backup and rollback: /root/backups/dob-account-deletion-20261006; rollback restores API/worker binaries and app.js, with no database rewind. Completed real deletions cannot be undone by binary rollback.
Deployment result and exact source hashes are recorded in ACCOUNT_DELETION_ACCEPTANCE_20261006.json and PRODUCTION_REVISION_STATUS.json after readback.
