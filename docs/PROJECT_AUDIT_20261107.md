# Project audit and provider/task boundaries — 2026-11-07

Source `84c3609b94fb22a3d984b03dedd393bc5cdd483d`, PR49 merged as `90db255745f306ea5c496de0d344dc0bee64e2d4`.
Canonical branch: `checkpoint/final-e2e-20260929`.
Both installations and public main/stable run `019def54477ceaae3e29bc7c7b7403db5258e1c9`; schema 207 is unchanged.
This documentation checkpoint follows the merge; git HEAD identifies its exact commit.
Acceptance time: 2026-11-06T23:18:36.819293+00:00. Private evidence: `/root/dob-project-audit-20261107`.

## Corrected behavior

- A proxy-required profile must reference the exact supplied proxy. Nil provider clients and implicit default transports cannot open proxy-required runtimes. Cancelled health work does not dispatch; oversized, trailing and malformed health JSON cannot become healthy evidence.
- Seven provider factories register through a composition package. Driver identity, typed nils and advertised capabilities are checked. Metadata reads are isolated copies. AWS, OVH, Kamatera and UpCloud declare their runtime store dependencies instead of provider-name branches in the opener.
- Task panics are contained without their values or argument dumps. Native recovery persists a sanitized failure and backoff; the stable panic marker prevents installer bypass from ignoring that backoff. No fabricated provider outcome or automatic replay is introduced.
- Every registered module enters its lifecycle even if a sibling fails first. Failure cancels siblings and waits for cleanup within a bounded grace period before the role returns. Existing systemd process ownership and durable reconciliation remain authoritative.
- Plugin-prefixed listeners stay under plugin publication ownership. Old legacy metadata cannot resurrect withheld plugin output. Empty or non-string legacy client IDs and literal missing-email values cannot create malformed links.

See [the provider/task contract guide](PROVIDER_TASK_BOUNDARIES.md). Compiled factory modules are not arbitrary dynamically loaded or sandboxed code; provider-specific mutation/authorization adapters remain explicitly reviewed.

## Verification

| Gate | Result |
| --- | --- |
| Focused new regressions | 15 roots, PostgreSQL/race, zero skips; run 42308529863 |
| Behavioral negative proofs | Six local baseline roots plus native recovery and output baseline failures reproduced |
| Module shutdown | 20 repeated passes |
| Full source | Go/vet/browser/native isolation and 395 mandatory database/native/race roots, zero skips; run 42308530050 |
| Backup restore | 207 migrations and transaction interruption passed |
| Primary identity regressions | run 42308529960 passed |
| Public source/install/reinstall | exact candidate passed; run 42309212345 |
| Atomic main/stable promotion | run 42310098727 passed with source/install gates and leases |
| Both live updates | 21 settings/ownership checks each, six running binary hashes and ten readiness checks passed |
| Panel assets | HTML and 32 assets each; public HTTPS/TLS verification passed |

The first full suite detected early sibling cleanup being skipped. The PostgreSQL test then detected installer bypass of the panic backoff. Both were corrected and the exact final revision passed. An initial output fixture lacked a deployment host and was corrected before its behavioral baseline proof was accepted. The complete gate then found an old inline-SSH test double missing its advertised compute interface; only that fixture was corrected, preserving the production contract guard. Failed attempts remain in CI/evidence; none is counted as a pass.

Gitleaks and comparison against current private values in both installations passed without logging values. No paid OpenAI call was made; the old unknown usage reservation remains untouched.

## Live acceptance and limits

All 3 active production accounts have fresh successful route/provider observations and no unresolved provider error at acceptance. Every route is proxy-required; explicit primary-country policy is respected. Saved output has 48 rows and no malformed legacy VLESS identifier in the checked forms. No runtime/task panic occurred in the acceptance window. Configuration, credential hashes, login identity, country/proxy policies and listener ownership passed preservation checks.

No cloud resources were created or deleted as a test. Live provider coverage is Vultr and Linode only. Other provider drivers and native browser behavior are verified in isolated environments, not universal live acceptance. This release does not claim a new Android touch/clipboard test or CAPTCHA bypass.

The structural inventory covers 38 modules/659 production Go files/632 test files. It is not a manual line-by-line review of all source. Manual work concentrated on factory contracts, runtime dependencies, routing/health, task recovery/supervision and output ownership. There is no zero-bug or measured 99-percent primary-IP guarantee.

The original offline cluster and workers remain quarantined. Physical corruption root cause and prior unknown cloud-create receipts remain unresolved; never restart them or replay uncertain operations as part of this audit. Do not reset the prior OpenAI ledger or duplicate its unknown request.

Next: continue evidence-led review of remaining ownership/resource paths and user-reproduced faults. Read this receipt first; do not repeat this completed deployment or clear saved provider errors manually.
