# Deployment SSH identity durability — 2026-10-25

Continuation baseline:3933c61256ee56cbb1a120dbef9446496faa0e3b.
The whole-project audit remains incomplete.

## Problem and resulting behavior

The application imported an SSH key before persisting its private key. A lost
response or crash could therefore discard the corresponding private material,
and recovery generated/imported another key. Initial independent regressions
target the old implementation in1e05d0acd096d4d3b00ec1eab4acc13d775f2228 (PR9).

The encrypted private key and a versioned intent now commit in one PostgreSQL
transaction before external mutation. Deployment and account ownership, provider,
external identity, credential reference and credential version bind that intent.
A row-serialized reserved-to-unknown transition authorizes exactly one create
dispatch per intent. It remains safe if the outer advisory lease is lost.
No database transaction stays open across a provider call. Only SSH-owned JSON
fields are merged; stale caller snapshots and unknown future fields are preserved.

Unknown outcomes use read-only inventory. Adoption requires one exact deployment
name and parsed public-key match within the original scope. Empty/partial reads,
invalid keys, duplicate matches and credential changes do not authorize creation,
adoption, deletion or key rotation. Existing orphan secrets from the old code are
reconciled as unknown rather than overwritten. Existing completed legacy identities
remain compatible. New secret insertion refuses collisions.

Inline providers and DigitalOcean's permission fallback retain the same encrypted
key. Recovery checks new intents even when the ready snapshot already has a key.
DigitalOcean and Vultr inventory adapters retain public-key material for strict
comparison. Vultr SSH pagination now uses its bounded endpoint/cursor helper.
A failed post-create verification no longer deletes an unproven Vultr key, and
missing creation IDs never become verification targets.

## Limits that remain explicit

This is at-most-one automatic import dispatch plus reconciliation, not a claim of
unconditional exactly-once remote execution. A crash after the durable unknown
fence but before the network call deliberately remains unknown if inventory cannot
prove the outcome. Credential rotation or an external account binding change
requires review of the old intent; it is never silently rebound to another account.
A missing/deleted or undecipherable private key is not automatically replaced.
No real cloud mutation is used by these fixtures. Live provider/fleet acceptance,
all remaining source inventory, multi-host control-plane fencing and sustained
fault/restore/load verification remain separate work.

## Verification status

The mandatory gate has213 top-level tests, including9 application SSH durability
roots and3 provider roots in this change. They cover pre-dispatch durability,
unknown-response replay, cancellation, failed final commit, reservation rollback,
reserved/unknown crash windows, concurrent callers, stale snapshots, secret
collisions, stale proxy generation, credential rotation, exact ownership matching,
legacy orphan adoption, inline/fallback handling and provider inventory/verification.
Independent CI is pending; required skips cannot be counted as passes.
No schema migration, credential setting or account policy is changed.

Default live release remainsa59597c4; production remains5dc2c256 with a disk-reserve
block. Both passed readiness with all six services active at14:43UTC. Original
services remain quarantined. The prior receiptless OpenAI request remains
unreconciled; no new generation request is submitted.
