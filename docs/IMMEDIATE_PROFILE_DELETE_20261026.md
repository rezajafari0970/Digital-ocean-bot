# Immediate profile deletion — 26 October 2026

Recorded 2026-10-26T01:10:01.614981+03:30.

## Result

A committed Delete now removes the profile from the active Configs catalog
immediately, even when an account credential is invalid or a remote server is
unreachable. Existing publication predicates remove the profile from output and
subscriptions in the same committed desired-state transition.

Previously, the active catalog waited for every remote listener to be confirmed
absent. It now excludes delete_requested_at as soon as the request commits.
The response distinguishes state=removed from cleanup_state=pending/complete.
The UI accepts only a receipt for the exact reviewed IDs, removes accepted cards
before another read, invalidates older reads, and filters stale snapshots.
A failed write does not hide a profile; a partial batch retains revision conflicts.
The batch selection shares the updated catalog even when the follow-up read fails.

Remote identities, durable deletion intents, confirmation evidence and occupied
port reservations remain available to the cleanup workers. The existing server
details endpoint still exposes remote removal progress. This is immediate removal
from active management and output, not a claim that an unreachable server was
physically changed or that historical audit/cleanup records were erased.

## Delivery and tests

- Source 4c8a3604d44ac86d8ce536d186b1fc875a75d090; PR 15 merged as c539db0a8e3ac82ae07dd1f3b4f23f12b3a90767.
- Public main/stable and both live instances: 11deda501628098630e9d728fa6876774a0ae0bf; schema 197.
- Full source CI 40235166589: 232 required roots passed,
  zero required skips; Go/static, real browser, native transport/race and isolated
  backup restore/interruption checks passed.
- New PostgreSQL regression uses TOKEN_INVALID and an unavailable native server:
  single and batch removal disappear from fresh API reads, retries are idempotent,
  revision conflicts and unrelated profiles survive, stale updates cannot revive
  deleted profiles, and unconfirmed remote ownership/cleanup is retained.
- Real mobile/desktop browser regression covers failed refresh, a held stale list,
  failed Delete, partial batch, pending profiles from an older response, and a
  wrong-identity receipt. No write is replayed automatically.
- The first CI attempt stopped because an older browser fixture still returned
  the previous deletion response. That fixture was updated to the new contract;
  failed run 40234005237 is retained. Product source was unchanged by that fix.
- The next gate (40234321467) caught ambiguous UUID/text parameter typing in
  the new test fixture. Explicit UUID typing fixed the fixture; its full gate
  was rerun. No validation assertions were removed or relaxed.
- Public CI 40235227957 passed fresh/repeated install and source validation.
  Atomic promotion 40235777101 passed with exact destination leases.
- Full export secret scans passed. Subsequent deltas were only the browser
  and PostgreSQL fixtures; their exact sources were scanned and every unchanged
  export hash was verified. See export2-scan-validation.json and
  export3-scan-validation.json in the private evidence directory.
- Both live installer attempts exited zero and each passed 15 policy/ownership
  checks, three running artifact hashes and five readiness checks. HTTPS version,
  normal TLS validation and served UI assets match the exact release.

## Live acceptance and remaining work

Active Configs profiles at acceptance:
1.
Detached background-cleanup profiles:
3.
Unconfirmed remote listeners:
3.

The three previously requested deletions no longer belong in the active Configs
catalog. The existing Tcp reality 8443 profile was outside the deletion request;
its settings and ownership are preserved by the rollout checks. No new
production profile was created or deleted as a test.

The three remote listeners still require valid authorized AWS credentials or
server reachability, or independently verified provider termination. Their
pending status is deliberately not converted into false remote success.

Fresh pre-update backups and receipts are under
/root/dob-immediate-delete-20261026 on the recovery host. One superseded routine
archive, 617280570 bytes, was copied to the existing archive host, destination and
source SHA256 were checked, and a durable archive pointer was written before
releasing the original space. Current and incident backups remain protected.

The original corrupted cluster and project services remain quarantined. Its
physical corruption cause and the comprehensive remaining module audit are still
open. No new paid OpenAI request was sent; the previous unknown request/usage
access restriction remains unresolved.

Do not repeat this accepted deployment. Continue remaining module audit using its
existing inventory; recover authorized remote access to complete native cleanup.
