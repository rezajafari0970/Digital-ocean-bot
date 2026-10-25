# Project Context
This repository is the durable source of truth for the DigitalOcean + Vultr automation project. Chat history is not the sole project record.

## Core rules
- Keep provider integrations modular and independent.
- DigitalOcean and Vultr capacity/account logic uses provider-specific authoritative data.
- Sanaei/x-ui live reads/writes use its API where implemented; Output must not depend on SSH reads.
- Output is live/fast rather than stale cached data.
- Proxy/account/provider modules remain independently testable.
- CAPTCHA/2FA challenges are completed interactively; authorized sessions may then be reused where appropriate.

## Continuity protocol
Before ending a development session: update PROJECT_STATE.md and HANDOFF.md with branch/HEAD, tests, deploy state, migrations, blockers, and exact next action; commit and push. Never store passwords, API tokens, private keys, cookies, or session secrets in these documents.

At a new chat/session: read PROJECT_CONTEXT.md, PROJECT_STATE.md, HANDOFF.md, then verify git status/HEAD and production service state before changing code.

## Current continuity entry — driver audit continuation 2026-10-25

Read the newest PROJECT_STATE.md and HANDOFF.md entries and
docs/DRIVER_AUDIT_20261025.md with its inventory/acceptance JSON. Implementation
640935c5 merged as 5ae2fd7c; public main/stable and default installation run a59597c4.
Exact source 201-test CI, independent installation and preservation checks pass.
Production 5dc2c256 remains disk-reserve blocked; original database/application/
publisher quarantine is unchanged. This audit is INCOMPLETE (88 files fully read,
including 76 of 1,218 baseline Go files).

Next address durable deployment SSH-key/intent reservation before provider import,
with scoped reconciliation and crash/concurrency tests. Do not confuse the new
driver single-call validation with complete end-to-end SSH import idempotency.
Continue per-module coverage and isolated restore/fault/load verification.

The existing API key is private on the original host. Its schema 2 authoritative
ledger preserves one unknown request andUSD 1.501011 reservation under the
unchangedUSD 2/three-attempt cap. No new paid generation was made in this continuation.
Provider usage must be reconciled before further paid reviews; never reset,
duplicate or override the ledger. Permanent uptime, arbitrary unknown-bug repair,
complete observability and multi-host active/active safety are not certified.
