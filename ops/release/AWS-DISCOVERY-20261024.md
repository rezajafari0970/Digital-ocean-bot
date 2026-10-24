# AWS discovery form correction — 2026-10-24

Baseline source ba81bf2a82438d52655bce46769fd346f0a6772f; baseline deployed/public
272c46317edff036c32e302c8adce4f0ff3dcda7.

The complete 50-second screen recording was inspected at one-second intervals.
Its audio track is silent. It shows duplicate additional region selections,
two empty choices (Select and None), and old subnet options remaining selectable
after changing the primary region. Server-side duplicate rejection was correct;
the discovery UI lacked the uniqueness rule already used by random region fields.
The previous mobile fixture exposed only one extra region without the primary
region in its options, so it did not cover this failure.

The AWS UI adapter now normalizes all five region slots, preserves primary
ownership, disables already-used options, and clears region-owned subnet choices
when their region changes. Disabled stale subnets cannot enter another preview.
A pre-request normalization also handles restored/programmatically changed values
without change events. The generic renderer creates a placeholder only when the
provider has not already supplied an empty option. Provider credentials and
server-side rejection boundaries remain unchanged.

The mobile fixture now has all four optional slots and multiple primary regions,
and rejects any duplicate region or stale subnet reaching its preview endpoint.
It exercises primary/extra conflicts, None removal, stale-state normalization,
reload, scheduling draft preservation and explicit network consent.

Final validation and rollout evidence follows.
No new OpenAI API calls. Prior ledger: 3 calls, 6,809 tokens,
USD 0.05716284695 estimated; three-call cap respected.

## Final validation and deployment

Implementation source: 1f833cda0f6d7d086d3133bbf9d330bcb3ce3e17.
Public main/stable and both installed instances:
ae5f2e8886fb9dd2dcb5e161aacb44847cfe9445.
The new browser regression fails against the previous static assets, identifying
both duplicate empty options and selectable duplicate primary regions. It passes
against the final assets with authorization popups both available and blocked.
All 156 mandatory isolated database/native/race tests passed without skips.
The exact exported source also passed the complete normal Go suite, vet,
installer/update/host-protection checks, live-secret scans and gitleaks.
An initial broad test command inherited the opt-in browser environment and
encountered missing /snap/bin/chromium in unrelated legacy fixtures; this was
corrected in the runner, without changing or weakening product tests.

Exact candidate source and independent installation, including repeat install:
https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40070028680
Atomic main/stable promotion with both old-head leases:
https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40070704533

Default verification completed at 2026-10-24T06:41:16Z and production at
2026-10-24T06:48:09Z. Every instance passed all five readiness checks and all
14 preservation checks. All three processes in each instance match its build
manifest, with zero restarts. Schema remains 000195_provider_snapshot_latest.
All 5,008 pre-existing profile identities and all 77 Trojan identities were
unchanged; four new profile identities appeared through normal operation.
There were no unexpected identity, account, credential, policy or key changes.

The public HTTPS panel returned the new revision at 2026-10-24T06:48:14Z.
Its AWS adapter, app JavaScript and stylesheet match the release source bytes.
Both integrity/backup timers remain enabled and active; no deployment is pending.
The fresh production backup was sealed, verified, checked for matching instance
configuration and retained through an additional hard link, avoiding a redundant
large dump while preserving independent retention. The normal locked staging
and deployment functions ran unchanged. Completed, reproducible Go caches were
cleared after staging, before the existing disk-reserve guard; no guard was lowered.

Two older task-specific database dumps remain recoverable in the encrypted
release-checkpoints snapshot recorded in the evidence JSON. Both were read back
completely and hash compared before their loose copies were removed. Recovery
references were written beside their original paths. Existing rollback archives
and normal backup retention were preserved. The legacy controller remains
quarantined and was not used for builds or database operations.

The API ledger remains unchanged: zero new requests or cost. The exact hashes,
aggregate preservation results and backup recovery references are in
AWS-DISCOVERY-20261024-evidence.json.

Fresh read-only saved AWS account count: 0. Live customer AWS create/delete was not performed; mobile and API fixture results are not represented as a live AWS account test.
