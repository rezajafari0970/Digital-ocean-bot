# Apply transport settings to existing listeners — 2026-10-15

The affected phone profile belonged to an older listener. Saving new-listener defaults did not update that listener's pinned Host, Flow or FinalMask, even when new clients were added to it.

The Configs transport editor now offers applying the saved settings to existing eligible managed listeners, displays per-listener progress, and provides retry and restore actions. Existing APIs retain opt-in apply behavior; the editor defaults its explicit apply-existing checkbox on. A bounded panel list permits a first-listener rollout.

Migration180 introduces durable application jobs and encrypted native backups. Application drains existing client mutations, serializes against other managed changes, preserves identities, quotas, expiry, REALITY keys and unrelated native settings, prepares the verified runtime when required, writes and reads back native configuration, restarts the core once with a durable journal, and only then promotes the listener pin and replaces old output snapshots. Legacy unpinned TCP listeners can be adopted. Unknown outcomes retain an output/client fence and require retry or a journalled restore; they are never reported as active. A bounded prewrite drain releases admission if old client work cannot complete.

Runtime acceptance compares the running executable and replacement process identity, listening socket ownership, exact file transport/REALITY/encryption/FinalMask configuration, and active core user membership through the local API. A saved file or panel label alone is insufficient.

Verification:
- Full project suite with isolated PostgreSQL and real Chromium passed.
- Focused race, vet and diff checks passed.
- Disposable Ubuntu24 with actual native panel/core passed host/Flow/FinalMask migration and restore while preserving users and limits.
- Negative runtime checks reject an unchanged process and a user absent from the running core even when the saved file still contains that user.
- Regression coverage includes lost write acknowledgements, one-shot restart recovery, retirement after unknown writes, bounded drain, blocked-job restore, and inherited unknown-state retry fencing.
- Six actual external reviewer responses were used. R1–R4 required changes; R5 approved the final retry correction; R6 approved the standalone browser-test correction after a stale disabled-selector expectation was found and fixed. All 26 reviewed product/test files are hash-frozen. Total usage 162022 tokens; actual billed money unavailable.
- The exact pinned FinalMask runtime artifact remains 5793567513420e536bdfca28464dbe831d13caa2. Fixture installation and executable receipts match that artifact.

Browser Dialer remains unavailable with REALITY. Its saved preference is shown honestly as inactive and cannot be selected as though it would activate this combination. No phone app/TLS endpoint changes or phone connectivity claim. UDP/QUIC masks do not act on this TCP HTTP/2 transport. Existing phone subscriptions require refresh after successful application. Core restart may briefly interrupt connections.

Normal gated publication, secondary/primary updates, first-listener application and remaining eligible fleet application are pending at this source checkpoint. Primary saved policy revision6 (aparat.com, XHTTP stream-up, VisionUDP443, FinalMask both) and secondary revision3 are preserved. No provider purchases, unrelated IP changes or client credential rotation.
Private evidence: .local/apply-evidence and /root/backups/dob-transport-apply-20261015.

## Live acceptance correction

The first public release23268f6 exposed API database pool starvation under the real primary fleet:64 collectors pinned connections for the new config lock and waited for nested SQL. No listener jobs had been queued. Primary was restored through the normal installer to approved0219700 and readiness5/5 recovered; schema180 remains. Output admission now reserves pool headroom using min(8,maxOpen/3). Actual PostgreSQL regression fails before and passes after at pool sizes6 and24; affected race, full isolated PostgreSQL/Chromium suite, and vet pass. R7 reviewer PASS. Total task7API responses 169138tokens; billed money unknown. Corrected normal release and live application remain pending at this checkpoint.

## Constructor integration correction

R7 live acceptance found New still eagerly constructed semaphore64, bypassing the lazy DB-aware limit. No transport application occurred. Primary was restored to approved0219700 a second time. Final correction removes this constructor preinitialization and changes the PG regression to instantiate the actual constructor. It fails before and passes after at pools6/24, with final affected PostgreSQL/race/vet PASS and R8 reviewer PASS. The full isolated PG/Chromium R7 suite remains the broad evidence immediately before this two-line wiring correction; it was not repeated. Normal publisher full tests/exact Ubuntu24 and live acceptance follow. Task8review responses 177534tokens; billed money unknown.
