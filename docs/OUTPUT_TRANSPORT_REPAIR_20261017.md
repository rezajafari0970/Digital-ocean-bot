# Output transport lifecycle repair — 2026-10-17

Status: accepted for the recorded cohort and current operator policies. Correction installed on both service hosts; final evidence and continuation checkpoint saved.

## Proven cause and correction

The operator deleted all clients/inbounds and changed creation defaults from TCP REALITY to XHTTP revision 8. A surviving listener's pinned transport is intentionally independent of creation defaults; output must describe actual native transport. Two defects were reproduced against the prior source with isolated PostgreSQL: verified delete-all retained obsolete listener pins, and apply-existing could adopt/queue stale inventory during QUEUED, RUNNING or PAUSED cleanup. Recreated listeners therefore reused old transport, while conversion jobs could attempt deleted listeners and fail before writing.

The installed correction retires pins and active transport journals only after a fresh native observation proves all clients and inbounds absent, or provider deletion is confirmed. Backups and historical receipts remain intact. Cleanup publication/completion and transport admission share the admission fence. Queue adoption, selection and execution exclude unfinished cleanup. A historical Restore cannot cross successful deletion into a new listener. Unproven disappearance and surviving listeners retain their pins. No URI type was blindly rewritten and no health/admission guard was bypassed.

## Operator changes preserved

Creation remains XHTTP revision 8: stream-up, 4096-byte path, Vision-udp443 and saved Extra/FinalMask settings. Publication options remain independently saved with include_xhttp_extra=false and include_finalmask=false. Connectivity checks use the actual published form, including those omissions.

At 00:48:00 UTC the operator reduced Residential from target 3/revision 15 to target 1/revision 16. Direct remains target 1/revision 11. Acceptance uses these current policies rather than the historical 1+3 target. Manual bootstrap users are not deleted to force profile counts.

One listener was explicitly restored to TCP: panel 38d91062-38c2-4cb6-af5e-65500352c196, job b3867942-2615-4f99-86a3-b0b45b1397f8, rollback_of a5b9c639-d2ea-43e6-b7f9-5bb6cd8faf64. It was requested at 00:37:40 UTC and completed at 00:51:42 UTC with CORE_VERIFIED_PIN_PROMOTED. Its target is TCP, despite the unchanged XHTTP creation default. This later per-listener Restore was preserved. Both its Direct and Residential published TCP links passed HTTPS-through-proxy checks. Its presence is explained by the explicit Restore, not stale export. Earlier commentary saying no TCP remained was incorrect and is superseded by this evidence.

## Validation and deployment

Product fix: ab7989e204dfa7909bcd7c44581a542009b2a3cb. Test fixture follow-up: cc2bd18eed7969cc74aa236e889eeb9791f2b058. Installed public release on both hosts: 4356ba77a28ffe548a2b30f538144375b67101e2. Schema remains 000182_fleet_client_coverage. The native core remains dob-fm-0.2.2.

Expected regression failures were reproduced for stale pins, unfinished cleanup admission and deletion/rollback boundaries. Focused PostgreSQL race tests and affected-package vet passed. A broader race/database run passed the transport packages but failed three admin fixtures: two strict persisted-environment guards rejected the isolated Unix DSN, and an old TCP fixture omitted mandatory core preparation. After using an authenticated loopback fixture and asserting core-preflight refusal without a write, all three failures and the deletion/rollback regression passed focused race reruns. The original broad failure is retained; it is not relabelled as a fresh fully passing database suite. The initial missing-role fixture setup error is also retained separately.

Publisher offline validation, build/vet/frontend/install gates and main/stable CI passed. Final CI runs: 38987269898 and 38987270078. Both service hosts passed five readiness observations each, all six running executable hashes matched, and automatic restart counts were zero. Accounts, environments, creation/output/protection policies were preserved during installation. Retained-pin changes were accepted only when independently explained by successful native-verified transport jobs. Protection remained operator OFF revision 51.

The first primary deployment attempt stopped before stopping services because disk reserve was insufficient. Under the publisher lock, 4,862 MiB of reproducible build cache was reclaimed; a separately recorded retry succeeded. No production backups were removed. Final private backup directory on each host: /root/backups/dob-output-transport-20261017-final.

## Live verification and interpretation

Recovery used the standard revision-fenced, per-panel apply API (32 initial and 10 follow-up admissions), followed by normal workers. Active/unknown mutations were not resubmitted. A bounded extra standard RunOne driver retained the global executor fence; its many no-op attempts are not represented as conversions. Historical FAILED_BEFORE_WRITE receipts were preserved.

The final read-only native audit covered 90 panels: 89 XHTTP running cores verified, plus the intentionally restored TCP listener. All 90 had zero native pin/URI-field discrepancies. Checks include native client membership/enabled state, endpoint, REALITY public key/SNI/short ID, encryption/flow and XHTTP path/host/mode/Extra. XHTTP runtime verification checks the immutable core digest, executing image, bound listener, native active-user API and core config validation. SHA-256 hashes of actual publication forms are compared with authenticated output and existing public shares; raw VLESS credentials are not persisted in acceptance artifacts.

Diagnostic corrections are explicit: the post-restart verifier's startup-file client-membership precondition is unsuitable after live AddUser/RemoveUser. The private read-only auditor skips that historical startup comparison while retaining the exact live core API user/flow comparison. Production post-restart verification is unchanged. Initial Residential example.com and ipify.org probes were blocked by the established client allowlist; they are not positive-connectivity failure evidence. The permitted BrowserLeaks destination is used for Residential acceptance. The initial full permitted run had two connection failures; targeted repeats passed and both first failures remain recorded. No claim that a future upstream request cannot fail is made.

Every HTTPS acceptance probe uses the actual output URI, validates the local client configuration, forces the local HTTP proxy with --noproxy '', and verifies curl's remote peer is 127.0.0.1. Direct uses example.com; Residential uses browserleaks.com/ip. Probe traffic is bounded, two concurrent requests at most. New fleet arrivals are checked separately; manual bootstrap output and profile creation are asynchronous. Real provisioning, expiry/deletion and provider failures remain visible.

## Evidence and continuity

At 01:04:30 UTC (04:34:30 Tehran), all90 healthy servers appeared in each output: Direct90, Residential90 and ALL270. The public shares matched those counts. Every actual output hash matched native evidence; each of those90 hosts passed HTTPS through both profile classes. Across full/catch-up audits91 distinct panels were checked (90 XHTTP running cores and1 explicit TCP Restore), with zero native/URI discrepancy. Three observations had zero missing healthy hosts, zero invalid links and zero profile target deficits. The first sample temporarily classified9 provider-unavailable servers; the next two recovered to90 healthy. A later dashboard snapshot at01:04:52 UTC showed91 active,1 inactive entirely due to provisioning,1 pending deletion,0 broken,0 degraded and0 panels needing attention. These are separate observation times and are not presented as simultaneous totals. Exact counts, time window and current-host probe coverage are recorded in docs/development-jobs/output-transport-20261017.acceptance.json. Per-stage history is in docs/development-jobs/output-transport-20261017.json. Private receipts and diagnostic helpers are under /root/projects/dob-output-transport-20261017/.local; they contain no raw share tokens in printed reports. The repository checkpoint, HANDOFF.md and PROJECT_STATE.md identify the resumable outcome.

OpenAI model API requests/tokens/cost: 0/0/0.
