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
