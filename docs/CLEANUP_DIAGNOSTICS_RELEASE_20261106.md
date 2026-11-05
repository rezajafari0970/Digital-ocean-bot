# Background cleanup statistics and provider diagnostics — 2026-11-06 Tehran

Accounts entering background cleanup are excluded immediately from active account health, provider totals, server totals/details and active deployment counts. Their retained cleanup evidence remains visible under Background cleanup.

The bulk local-removal button freezes the current cleanup target list and obtains explicit acknowledgement that cloud resources may still exist and incur charges. It retains account, operation, IP, recovery and consent-scope fences, prevents duplicate clicks, and reports removed/queued/review results per account. No live cleanup account was purged during rollout acceptance.

Provider failures preserve bounded HTTP status, native code, safe operation, error class, guidance and retry evidence when available. Failure history is persisted atomically with the accepted observation, deduplicated for identical recent errors, and displayed in account details. Cleanup cards display provider restrictions and independent network/creation failures. A Vultr 401 does not establish whether the API key, API access or selected exit IP was rejected.

## Verified release

- Tested source: a0228c43d2322dc07b0fa72fab509016a4883fcc; PR39 merged as 0abbdfdc160a0643d946cd6672793b9c6ea8b6ac.
- Public main/stable and both installed runtimes: 8d28fee2d356cb15a587dabb6a003a8a18863aaa.
- Source CI 42128311851: 365 mandatory roots passed, zero selected skips; native/race and disposable restore gates passed.
- Public CI 42128634651: source checks and Ubuntu fresh/repeated-install identity checks passed.
- Atomic promotion run 42129174773 verified both exact CI revisions and main/stable leases.
- Exact export manifest and both live-secret scans passed; gitleaks reported no leaks.
- Each instance passed 18 policy/identity comparisons, all running executable hashes, all five readiness checks and HTML plus 25 assets.
- Real production browser acceptance checked bulk cancellation without removal, visible cleanup issues, reload and six viewport widths. Actual bulk mutation tests ran only in disposable fixtures.
- Final observation: 2026-11-05T23:39:11.995138+00:00. Schema remains 206; no migration was added.

## Continuation

Private evidence and idempotent deployment receipts remain under /root/dob-cleanup-diagnostics-20261106 on the active recovery host. Do not rerun completed update intents. Begin any later work with current live state rather than treating this dated receipt as current.

Provider-side restrictions/authentication failures and unresolved historical create outcomes are not proof of application failure or cloud absence. Do not replay unknown creates, forge terminal absence or discard their evidence without explicit scoped operator consent. No saved AWS or UpCloud account was present for live customer-account checks; their diagnostics passed synthetic/provider contract regressions. The original damaged cluster and duplicate workers remain quarantined.
