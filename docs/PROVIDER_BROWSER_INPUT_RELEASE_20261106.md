# Provider browser interaction release — 2026-11-06

Accepted at 2026-11-06T16:25:00.267222+00:00. This release supersedes the interaction description in the earlier rendering release.

## Exact release
- Tested source: 737b5bff3889d5facadb8d9950647860135b1769; canonical merge: f1df969351b12128dd072246cd52418c731915b4 (PR46).
- Public main/stable and BOTH runtime instances: fa006e38b711c5645620cda9ec874b804eff62fb; schema206 unchanged.
- Source CI 42243922475: 373 mandatory roots, zero skips, exact source/browser/native/race/database/restore gates passed.
- Public CI 42244163579: test, exact exported manifest, Ubuntu install and reinstall with identity preservation passed.
- Atomic promotion 42245835942 passed with exact source/candidate pins and destination leases.
- Both installations passed18 preservation comparisons, five readiness checks and three running binary hashes. Shared browser runner hash verified. Production retained its first failed comparison: one already-disabled/deleting OVH account, its schedule and319 unassigned listener rows disappeared while the original runtime was still installed. Archive-only inspection proves all319 rows belonged to that account in the earlier sealed backup and all exact identities were already absent in the native sealed backup completed before publication. The derived comparison excludes only these proven pre-publication removals; common values remain strictly compared. Two listener transitions separately passed the existing expired-server retirement checks. This evidence does not establish a final cloud-deletion outcome or authorize replay of unknown creates.
- HTML and28 assets match on both instances and the user's HTTPS gateway with normal TLS verification.
- The first production attempt stopped before service swap at the unchanged retention-space guard. Two reproducible Go build caches were reclaimed (908MB); no account data or backups were deleted. The same verified stage resumed through the original integrity, backup, admission and space guards. Both final accepted updates are complete; the failed attempt and its receipt are retained. Do not replay deployment intents.

## Fixed behavior
A touchscreen tap is delivered as one native press/release action without a screenshot round trip between the phases. Coordinates are captured before the local keyboard can change the viewport. Cookie overlays exclude covered fields from keyboard targeting. Mouse drag/hover and scrolling remain native, with queued motion coalesced.

The currently focused writable input or textarea is mirrored into a native local editing control. Its value is retained for IME, selection, copy/paste and undo. The original one-character sentinel was cleared after each keystroke and could not support ordinary clipboard operations. Real fields now support Persian text, replacement, password masking, multiline editing and explicit clipboard controls; the paste dialog also works without the secure-context Clipboard API. No automatic clipboard read was added.

Edits identify the active node and expected current value, remain bounded, authenticated and transient, and use native Chrome insertion. Field values are not logged or persisted by this feature. Action/start responses are non-cacheable. Correct native modifier chords replace sequential Control/Shift key presses. Queued edits are captured before navigation; stale selection/key-up events cannot close the new editor after Tab. Input typed during a focus transition is buffered for the destination field. Android's keyboard uses the resizes-content viewport behavior.

The existing isolated full Chrome, proxy/country fences, cookie separation, high-DPI PNG rendering and Desktop mode remain. No provider-side automation, stealth scripts or automatic security-challenge solving was introduced.

## Verification
The old assets fail the retained-text regression. A separate latency regression reproduces the old post-Tab editor closure; the corrected source passes.
The native test follows actual touchscreen/clipboard input through the standalone page, isolated Go worker and real browser DOM, with120ms response latency. It passes:
- Cookie dismissal and rejection of keyboard targeting behind the overlay.
- Keyboard viewport resize and trusted native Persian input.
- Real clipboard copy/paste and partial text replacement.
- Rapid typing followed immediately by a button tap.
- Password fields, buffered Tab, Shift+Tab and multiline input.
- Touchscreen interaction in Desktop mode and clearing local editing state on End.
Wrong-node and stale-value edits are rejected. Browser/proxy/cookie and resource-isolation regressions continue to pass.

The installed production UI was also exercised through the normal-TLS public HTTPS gateway against the anonymous Vultr login page. Synthetic email/password text, cookie buttons, native clipboard, keyboard resize, Tab/Shift+Tab, touchscreen scrolling and Desktop mode were checked without submitting login. The first long-running DPR3 audit client coincided with host-wide memory exhaustion: the kernel reported global_oom and killed the audit Chrome process at16:15:18UTC. Both production worker units also exited once during that pressure interval and automatically recovered by16:16:16UTC; final readiness and binary checks passed. This was not merely a test cgroup-local OOM. The failed audit was discarded; its temporary operator/profile were explicitly removed. Do not run high-DPI or heavy native tests on this4GB no-swap host; use disposable CI. The bounded public test usedDPR1; high-DPI rendering remains covered by the native fixture and earlier rendering gates. The exact observed checks are in the adjacent JSON receipt. Temporary audit credentials and browser profiles were removed; no provider login or cloud-resource mutation was used for acceptance.

## Boundaries and continuation
This is an isolated remote browser. It does not turn the phone's address bar into the provider's origin, and cannot promise identical behavior across every website/device. Physical Android acceptance and real provider-account login were not performed. Cloudflare or other security challenges may still require the user's normal interaction.

Native editing mirrors standard input/textarea controls, including accessible same-origin frames. Rich contenteditable editors and cross-origin frame editing do not have full native clipboard parity. Upload/download remain unsupported. Session duration, background expiry, two-session capacity and the1280MiB aggregate memory budget remain unchanged.

Preserve the original-cluster quarantine, credential and country policies, listener ownership and unknown-create receipts. Do not replay unknown cloud creates or start duplicate workers. Earlier cleanup statistics, bulk local cleanup removal and provider diagnostics remain included; no live bulk purge was executed in this work.
Private operational evidence: /root/dob-browser-input-20261106.
