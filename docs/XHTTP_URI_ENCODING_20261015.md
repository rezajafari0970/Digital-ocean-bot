# XHTTP URI encoding repair — 2026-10-15

The phone failed before establishing any connection: noGRPCHeader reached the core as the string "+true", not boolean true. PostgreSQL jsonb spacing was encoded by Go url.Values.Encode as literal plus. Official v2rayNG Utils.decodeURIComponent intentionally preserves literal plus, so this was an exporter defect; prior form-decoding checks hid it.

The VLESS exporter now encodes spaces as %20 in query and fragment after form encoding. Real plus remains %2B. Extra JSON types, literal plus, Unicode, 4096-byte paths, encryption, Flow and FinalMask survive strict component decoding. Existing listener pins and creation defaults are not changed.

Evidence:
- Both legacy and Vision/FinalMask regression cases FAIL on baseline and PASS after correction.
- Real PostgreSQL jsonb/shared-output test and focused race tests PASS.
- Actual production legacy and Vision/FinalMask URIs reproduce the failure. Corrected exports pass strict decoding and official Xray 26.9.9 configuration tests.
- A real reviewer API call returned PASS. Its optional missing-extra assertion was added and PostgreSQL race regression rerun successfully. No runtime code changed after review.
- Package vet passed. Normal publisher/exact Ubuntu 24 and both-host updates remain pending.
- Official v2rayNG source inspected at e1b73f2fa8487dfc303da52d109719c1e32386b4: Utils.decodeURIComponent replaces literal + with %2B before URLDecoder; FmtBase uses this decoder. Android/Gson execution was not performed.
- Phone SSH is disconnected. No actual new phone connection claim.
- User log separately contains UDP port443 outboundTag=block; this client routing rule must be disabled to permit UDP443, independent of Flow.
- Existing imported profiles require subscription refresh or re-import of the corrected link.

One API response: 2597 input + 354 output = 2951 tokens. Actual money is unknown.
Evidence directory: .local/uri-evidence. Production readback artifacts contain secrets and remain private/untracked.

## Deployment acceptance — 2026-10-15T19:01:15.298405+00:00

Source cffd27319bb3c492c1c94c12bdbbed0d785ac09d exported public 0219700581c967f1fd6523ff8d047b0518ae3b25 to main/stable and both installed hosts/schema179. Normal publisher, secret/unprivileged tests, exact Ubuntu24 installation and both GitHub checks PASS. Secondary then primary updates PASS: 3 active matching binaries and readiness5/5 each; accounts/env/retained pins/creation policies preserved. Strict URI-component decoding of current API and existing shares ALL/DIRECT/RESIDENTIAL has zero invalid Extra/FinalMask/type or pin mismatches; primary ALL counts {"xhttp": 125, "strict_extra_valid": 125, "tcp": 5}. Secondary has no accounts/listeners, so its empty outputs are not live XHTTP proof.
Phone error was our form encoding of JSON spaces as +; fixed %20 preserves true booleans/numbers and literal %2B. Both actual legacy and Vision/FinalMask fixtures pass official26.9.9 configuration validation. Phone SSH remains disconnected; refresh subscription/re-import needed. User log has separate client UDP443 block rule. Primary creation policy was already TCP revision5 before this repair; preserved, not silently reset to XHTTP. Secondary remains XHTTP/UDP443/FinalMask revision3. Existing XHTTP pins continue exporting correctly. Current visible output has zero Vision+FinalMask links despite one retained matching listener pin; two saved actual production fixtures (legacy and Vision+FinalMask) passed strict decoding and official core configuration tests. Do not claim every feature is visible/enabled in current subscriptions. Browser Dialer remains saved/inactive with REALITY from preceding task. No new core/provider changes. Task1 API response2951tokens. See docs/XHTTP_URI_ENCODING_20261015.md and acceptance JSON. Other IP work paused.
