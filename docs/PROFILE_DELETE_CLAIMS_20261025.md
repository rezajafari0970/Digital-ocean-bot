# Profile deletion concurrency — 2026-10-25

Recorded 2026-10-25T20:55:40.307915+00:00.

## Delivered

Source candidate 858b104a5211c10ae13ebf1339c58f74a5b45bbb was merged through PR14 as
4679847f3d5725b7ffec23575b283dc20250e499 into checkpoint/final-e2e-20260929.
Public main/stable and both live instances run ec80c7c9d5f2e8eb8502e2f410b9fdde3b35c429,
schema197. No schema change was required.

The destination-claim query previously locked both a listener and its shared
profile row. Concurrent workers could return no work for independent panels of
the same profile. Claims now lock listener rows only. The profile fairness
timestamp update skips a locked profile so an editor cannot block independent
destination claims or invert the listener/profile lock order. Reconciliation
still rereads desired state. Per-panel writer locks, claim fencing, durable
mutation journals, remote readback and deletion evidence remain in place.

This complements the already deployed deletion priority and creation-worker
assistance. It does not turn a timeout into successful remote deletion.

## Validation

- Exact-head source CI40231487805: all231 required roots passed, zero required
  skips; full Go/static/browser checks, database/native transport/race gate and
  isolated schema197 restore/interruption drill passed.
- Focused CI40231482832: three race-enabled repetitions. Eight independent
  destinations sharing a deliberately locked profile must all be claimed before
  the profile lock is released; a further claim must not duplicate a lease.
- Public CI40231882469: source manifest, fresh install and repeated install with
  identity preservation passed.
- Atomic main/stable promotion40232099661 passed, then both destination refs
  were read back at the exact candidate.
- Export production-specific/default/gitleaks scans passed.
- Both final accepted live installer attempts exited zero. Each passed15 policy/ownership
  preservation checks, three running-binary hashes and five readiness checks.
  HTTPS frontend version/readiness and served UI assets match with normal TLS
  verification. Exact counts and timestamps are in the adjacent JSON receipt.

A synthetic PostgreSQL diagnostic (40231139681) used104 tasks, eight workers and
an artificial30ms delay. The shared-profile-lock baseline produced320 empty
claims in7.266s; listener-only locking produced zero empty claims in1.950s.
This diagnosed lock contention. It is not a live fleet latency benchmark and
does not measure network deletion or the final nonblocking fairness query.

## Pre-publication storage stop and recovery

The first production attempt stopped at the unchanged free-space guard before
publication. The old runtime remained active and no pending deployment was left.
One superseded pre-update archive (615107439 bytes) was transferred using the
existing restricted backup identity, destination and source SHA256 were verified,
and a durable archive receipt was written before releasing the source space.
Current and incident backups stayed local. The exact staged candidate was then
resumed through the unchanged integrity, space, identity and deployment guards.
The first failed receipt and successful resume receipt are both retained.

## Remaining remote deletion blocker

Three unretired listeners belong to one unreachable AWS server. The account is
TOKEN_INVALID; prior provider verification returned InvalidClientTokenId.
Fresh recovery-host probes to the panel and SSH timed out. Each of three deleting
profiles has one unconfirmed listener remaining. The UI retains pending cards
because remote removal has not been proven.

Restore the account's authorized credentials through Accounts and/or restore
network access, then let the existing durable deletion jobs finish and verify
their remote readback. Independently verified provider termination is also
valid evidence through the existing lifecycle. Do not delete database rows,
erase ownership or force retirement to make the count zero.

## Requested OpenAI API review

Maximum reasoning was requested, but no new paid generation was sent.
The existing bounded review helper refused before POST because one earlier
request has an unknown outcome and retains USD1.501011 reservation under the
existing USD2/three-attempt cap. A fresh usage read returned HTTP403.
Resolve usage/result with appropriate account access or the original receipt
before another paid review. Do not reset the ledger or duplicate the unknown
request. The implemented fix is supported by direct reproduction and tests,
not by an API review claimed to have completed.

## Recovery and handoff

Private rollout receipts and current pre-update backups:
 /root/dob-delete-claims-20261025 on the recovery host. Preserve these and earlier
incident archives. The original corrupted cluster and project services remain
quarantined. The corruption's physical cause is still unproven.

Whole-project audit remains incomplete; the previous recorded inventory was
269 complete files/31720 lines. This focused change does not certify every
module, permanent uptime or instant deletion on unreachable hosts.

Next: restore authorized access for three AWS deletions; reconcile the unknown
OpenAI request before another paid review; continue remaining application/provider
ownership and resource audit using its existing inventory.
Do not repeat this completed deployment.
