> Historical milestone. Current release, rollout and cumulative coverage: [scheduler continuation](SCHEDULER_DURABILITY_20261025.md).

# SSH identity durability, encrypted-record containment and restore verification

Updated: 2026-10-25T18:46:39.199013+03:30. Continuation baseline: 3933c61256ee56cbb1a120dbef9446496faa0e3b.
**Whole-project audit: INCOMPLETE.** Read the [coverage inventory](SSH_IDENTITY_DURABILITY_20261025.json)
and [acceptance evidence](SSH_IDENTITY_DURABILITY_20261025_ACCEPTANCE.json).
Search matches, partial reads and passing tests are not counted as full source review.

## Stage status

| Stage | Status | Evidence and limit |
| --- | --- | --- |
| Durable deployment SSH identity | Implemented, tested, merged | PR #9; product 4f2e595e3d7b686e2f80effa25cff3761893a270; merge 4104b382 |
| Malformed encrypted records and retained-key reads | Implemented, tested, merged | Negative reproduction, memory and isolated SQL regression |
| Combined source verification | Passed | CI 40205454182: 215 required roots, zero skips, full Go/vet/browser, 20 isolation repetitions |
| Isolated database restore and interruption | Passed, required in CI | PR #10; 5801e0d1fed3ce1158d7fd8f923ee993a3c7359b; merge be4036cff09dca3ab0e7bffe0702fe7cdbbe17f2 |
| Public installation release | Published | main/stable 329d51a88fe75e2bd4e6680e16061e8390e517df; fresh and repeated installation, login and identity preservation |
| Recovery default installation | Updated and verified | 215.20s bounded update; 14 preservation checks; schema 195 |
| Recovery production installation | BLOCKED by disk reserve | Still 5dc2c256; no upgrade attempted in this continuation |
| Original server | Quarantined | PostgreSQL, application and publisher inactive |
| Further paid API review | BLOCKED by unresolved request | Original ledger preserved; zero new generation calls |
| Entire source/architecture audit | In progress | 138 full current files; 105/1,218 baseline Go files |

## Durable SSH reservation and recovery

Previously, the application imported an SSH key before persisting its private key.
A lost response or crash could discard that private material; recovery generated
and imported another key. Negative CI 40203061020 at 1e05d0a reproduced a missing
private secret at dispatch and three creates after three ambiguous attempts.

The encrypted private key and a versioned intent now commit together before any
external mutation. Account/deployment ownership, provider, external identity,
credential reference and credential version bind the intent. Row locks follow
account, credential, deployment order. A reserved-to-unknown transition permits
at most one automatic create dispatch per intent, including after outer advisory
lease loss. No SQL transaction remains open during provider calls. Only SSH-owned
snapshot fields change; stale caller state cannot erase unrelated future fields.

Unknown results use read-only inventory. Adoption requires one exact deployment
name and parsed public key match in the original scope. Empty/partial inventory,
malformed keys, duplicate matches and changed credentials retain unknown status.
Legacy orphan secrets are reused and reconciled, never overwritten. Completed
legacy identities remain compatible. Insert-only secrets reject collisions.

Inline providers and DigitalOcean's permission fallback retain the same key.
Recovery validates new intents even if a ready snapshot has an existing key ID.
DigitalOcean and Vultr inventories expose public keys. Vultr SSH pagination is
bounded and endpoint-scoped; missing creation IDs stop verification. Failed
post-create verification no longer deletes an unproven remote key.

This is at-most-one automatic dispatch with reconciliation. A crash after the
unknown fence but before dispatch can leave an unresolved intent with no remote
key. Credential changes and absent inventory proof need review. Missing or
undecipherable private keys are not fabricated, rotated or silently replaced.
These fixtures do not mutate real provider accounts.

## Encrypted-record corruption boundary

A controlled negative test reproduced AES-GCM panics for nonce lengths
0, 1, 11, 13 and 128. A synthetic repository returning a valid other-owner record
also exposed a missing store-level ownership check. The SQL query already scopes
ownership; this is not evidence of a live cross-tenant incident.

A shared decoder validates retained key version and exact nonce length before
decryption. Store reads independently validate requested owner and identity.
Proxy, probe and residential readers now use persisted key_version instead of
only the active key. Account, proxy, probe, residential, Cloudflare and IP cleanup
readers reject malformed ciphertext/version/nonce without returning plaintext
or panicking. Retirement still rejects corrupt archives. No corrupt ciphertext
is discarded or claimed repaired.

Focused local verification passed five roots and skipped one SQL-only root.
The final CI executed the required SQL regression without skipping it. Its
isolated six-namespace fixture verifies retained old keys and corrupted records.
This contains malformed encrypted records; it does not repair physical storage.

## Isolated backup restore and fault drill

The new drill starts an unprivileged temporary PostgreSQL cluster with checksums,
private Unix sockets and no TCP listener. It discards inherited live database
environment variables and uses only disposable databases and a synthetic key.
The real migration runner applies all 195 migrations. A fixture seeds an account,
profile, encrypted credentials, event and unknown SSH intent.

A sealed custom-format dump and synthetic configuration key are restored into a
separate database. Canonical identity/migration digests, actual secret decryption,
unknown intent, sequence and pg_amcheck pass. A half-truncated archive is rejected;
a failed single-transaction restore leaves no partial application tables.
A SQL backend is terminated inside an uncommitted transaction; its inserted row
rolls back and durable identity remains unchanged. The temporary cluster is
stopped and removed in cleanup. No live database is accessed.

Standalone CI 40205257090 and combined CI 40205454182 both passed. The combined
workflow now requires this drill after the 215-root native/database/race gate.
This is a synthetic fixture drill, not restoration of the retained production
backup, sustained-load evidence, an RTO measurement or proof of physical repair.

## Verification and publication

Product CI [40204392265](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40204392265)
passed at 4f2e595e3d7b686e2f80effa25cff3761893a270. [PR #9](https://github.com/rezajafari0970/Digital-ocean-bot/pull/9)
merged as 4104b382. Final combined
[CI 40205454182](https://github.com/rezajafari0970/Digital-ocean-bot/actions/runs/40205454182)
passed at 5801e0d1fed3ce1158d7fd8f923ee993a3c7359b; [PR #10](https://github.com/rezajafari0970/Digital-ocean-bot/pull/10)
merged as be4036cff09dca3ab0e7bffe0702fe7cdbbe17f2. Its four operations/CI files are excluded by the public
export selector; it does not change the previously tested product.

The 1,735-file export was checked against both live configuration secret sets and
gitleaks before publication. Public [CI 40204626071](https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40204626071)
passed source validation and exact fresh/repeated Ubuntu 24.04 amd64 installation,
five readiness checks, ordinary login and preserved identity. Promotion
40204888284 atomically updated main/stable to 329d51a88fe75e2bd4e6680e16061e8390e517df; both actual refs were
independently verified. Canonical manifest SHA-256:
00187a138e73a09fd311eff1e3458d256d5a68b3606da9f9f8bb91816524f874.
The installer is available in the public stable branch.

No migration, credential setting or account policy changed. Earlier discovery
timeout CI evidence remains retained; its cause is unproven. Twenty unchanged
isolation repetitions passed, without relaxing timeouts or assertions.

## Live rollout and resource boundary

The recovery default updater completed in 215.20s under MemoryMax=1GiB and one
CPU. At 15:04:51 UTC all 14 preservation checks, three executing hashes and five
readiness checks passed; schema remains 195. Controlled service restart occurred.
There are no default profile/Trojan identities, so this is not production fleet
acceptance. At 2026-10-25T15:09:48.030306+00:00, all six services across both installations were active
with matching executing hashes, NRestarts=0 and five readiness checks each.

Production remains 5dc2c25678698dbbe9ac1cfee7d18701c19213e7. Read-only preflight
at 2026-10-25T15:09:48.291729+00:00 found 3,956,871,168 bytes free against a reserve
of 6,719,104,061; deficit 2,762,232,893 bytes.
Build and backup growth require additional margin. No pending deployment or
production update was started. The 60GiB virtual disk's partition already fills
the available disk. Unrelated application data and retained incident/rollback
archives were preserved.

The default updater's existing retention removed one expired verified runtime
and one expired verified backup. Manual cleanup removed only completed,
reproducible Go caches (448,622,592 bytes reclaimed in the measured cleanup).
Current stored integrity/backup reports are OK. The daily backup helper requires
5GiB free for its next dump; available space is below that threshold. This is a
future headroom risk, not evidence that a scheduled backup has already failed.

After restoring sufficient space, reverify current stable and CI, take a fresh
production preservation baseline and run /usr/local/sbin/dob-production update
through ops/release/run-bounded-update.sh with ordinary archive/integrity guards.
The retained /opt/.dob-release.bXqgHMgu is OLD 8d838efa, not current 329d51a8.
Do not deploy it under a latest-release label, bypass reserves or delete incident
evidence to force acceptance.

## Original host and bounded API review

At 2026-10-25T15:07:02.042843+00:00, original PostgreSQL/API/workers and publisher remained
inactive. Keep quarantine and do not run heavy tests there. The existing key is
private on the original server; it was neither printed nor transferred.

The authoritative schema 2 ledger still has one unknown request with
USD 1.501011 reserved under the unchanged USD 2/three-attempt cap. Actual tokens
and billed cost are unknown. Zero new generation requests were made here.
The earlier usage GET returned HTTP 403; it was not retried in this continuation.
Reconcile provider usage before another paid review. Never reset, duplicate or
override the ledger. No claim of a completed independent API review is made.

## Coverage and next work

Cumulative coverage: 138 full current files / 16,499 lines, including
105 of 1,218 baseline Go files / 12,577 baseline lines. This continuation adds
50 full files. Digests were reverified; the immutable baseline remains c41ee01a.
The remaining 1,113 baseline Go files are not certified as fully reviewed.

Continue worker/panel and remaining provider semantics with bounded queues,
cancellation, resource lifetime and ownership checks. Finish multi-host fencing,
bounded metrics/tracing, sustained load/fault verification, and an isolated
restore of a retained production backup on a host with sufficient resources.
Investigate the original physical corruption cause. Live AWS acceptance, selected
DNS capacity and gateway management remain open. Provider factories remain
compile-time and core setup still has provider-specific branches.

Permanent uptime, no future bugs and automatic repair of arbitrary unknown damage
are not established by these results. Private raw evidence is retained at
/root/dob-ssh-durability-20261025. This report and continuity files are committed
on checkpoint/final-e2e-20260929; git HEAD identifies the documentation checkpoint.
