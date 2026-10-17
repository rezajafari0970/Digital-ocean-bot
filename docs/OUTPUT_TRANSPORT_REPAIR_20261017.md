# Output transport lifecycle repair — 2026-10-17

Status: product correction implemented; release and live acceptance pending.

The operator deleted all clients/inbounds and changed the creation policy from TCP REALITY to XHTTP revision 8 while rebuilding. A retained listener pin is deliberately independent of creation defaults, so output must describe actual native transport, not blindly copy the current default.

Two defects were reproduced against the prior source with isolated PostgreSQL: verified delete-all retained per-listener transport pins, and apply-existing could adopt/queue stale inventory while cleanup was QUEUED, RUNNING or PAUSED. Recreated listeners consequently reused their old transport; conversion jobs could repeatedly try a deleted listener and fail before writing.

The correction retires pins and active transport journals only after cleanup proves native clients and inbounds absent (or confirms provider deletion). It preserves backups and historical receipts. Cleanup scope publication/completion share the transport admission fence. Queue adoption, queue selection and execution exclude unfinished cleanup. A historical Restore request cannot cross a confirmed deletion into a newly recreated listener. Existing listeners and unproven disappearance continue to retain their pins.

Validation: the original regressions failed with the expected stale-pin and cleanup-admission failures; the focused race suite passed; affected-package vet passed. A broader database/race suite is still running. An initial fixture setup failure (missing test-only role) is retained separately and is not counted as regression evidence.

Live recovery uses the standard apply API with immutable expected revision 8 and per-panel scope. It does not rewrite URI type, remove clients, disable safeguards or change the saved policy. 32 eligible retained listeners were queued in the first recovery batch. Ongoing operator conversion and server lifecycle remain active.

Native/output audits compare URI endpoint, client identity, REALITY public key/SNI/short ID, XHTTP path/host/mode/Extra and encryption against fresh panel state; running-core acceptance follows conversion. The post-restart verifier's preconditions are not evidence of failure on legacy TCP settings. No final fleet coverage or connectivity result is claimed yet.

Detailed stages: docs/development-jobs/output-transport-20261017.json. Private evidence is under this task worktree's .local/evidence. OpenAI model API requests/tokens/cost: 0/0/0.
