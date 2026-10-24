# AWS regional catalog and onboarding correction

Baseline private source: 06739aed93ba3dc8e6a89daee749079d543115b0.
Baseline public stable and both instances: 27257387077ff80e2e2a4d7539bc61d3bb7a2e06.

The screenshots exposed absent pricing, a regional-only account catalog,
duplicate optional selections, an implied bundled SSD, and a distant network
approval. The AWS factory now composes up to five separately scoped regional
drivers. Existing regional identities remain compatible. Explicit additional
regions pin their public subnets and managed network intent before save.
Inventory and identity search fail closed on any regional error. Network
journals are validated globally and cleaned up within their exact regions.

The catalog admits current x86_64 HVM/EBS types offered in the chosen subnet's
availability zone and compatible with all offered Ubuntu boot/ENA requirements.
Create revalidates the selected image and plan before any compute mutation.
EBS is separate storage: at least 12 GiB or the image minimum, whichever is
larger. The AWS UI adapter shows the instance ID, GiB/MiB RAM, CPU and a regional
compute price rather than inventing included SSD capacity.

Optional pricing uses the official AWS GetProducts endpoint, Linux shared
On-Demand filters, complete bounded pagination, positive finite USD hourly
rates, dated terms and conflict rejection. It is bounded to 12 seconds,
20 pages and 16 MiB. Compute-only estimates are not invoices or spending
approvals. Missing pricing permissions and unavailable prices remain explicit.
The account catalog cache remains credential- and version-scoped.

The mobile wizard retains scheduling edits across network reload, offers
None for optional plan/image choices, removes duplicate defaults, and places
explicit network consent beside Save. No automatic consent is introduced.

Validation so far: AWS race suite and mobile browser checks (authorization
popup available and blocked) pass. Full normal Go tests/vet and installer
unit checks pass. The final 156-case isolated database/native/race gate,
secret scans, exact candidate hosted installation and rollout preservation
checks are required before promotion. Final evidence is appended after execution.

A read-only production count found zero saved AWS accounts. Synthetic wire
and browser fixtures are not a live customer-account test. The legacy host
remains quarantined; no builds or database operations are run there.

OpenAI API: 0 new calls. Prior cumulative ledger: 3 calls, 6,809 tokens,
USD 0.05716284695 estimated; three-call cap already consumed.

## Completed release verification

Implementation source: 13ea6575efd492ae90e32176feea547f60021eef.
Public candidate/main/stable: 272c46317edff036c32e302c8adce4f0ff3dcda7.
Public tree: c5624f4f7f2761d560c8e7b55db8bae512dae8c8.
The exact exported source passed all Go tests and vet, the focused AWS race
suite, and 156 required isolated database/native/race regressions with zero
skips. Both mobile browser runs passed (authorization popup available and
blocked), exercising additional region selection, no duplicate defaults,
pricing labels, retained lifetime/desired-server edits and explicit consent
beside Save. The production and default live-secret scans and gitleaks passed.

Independent source and clean installation jobs both passed for that exact SHA:
https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40066053925

The GitHub-hosted atomic main/stable promotion passed and both resulting refs
were independently read back at the candidate SHA:
https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40066721562

This publication does not depend on the quarantined legacy controller.
Its remote-command connection was offline, although the panel gateway remained
reachable. Existing authorized SSH access from the recovery host was rejected;
no alternate credentials were probed. The healthy controller and GitHub
connector handled this release. The legacy local publisher state is stale
and must not be resumed as a current source of truth.

## Completed deployment and preservation

Both independent instances now run 272c46317edff036c32e302c8adce4f0ff3dcda7.
Default verification completed at 2026-10-24T05:41:56Z; production at
2026-10-24T05:51:29Z. All three processes per instance match the release
manifest hashes, have zero restarts, and pass all five readiness checks.
Schema remains 000195_provider_snapshot_latest. All 14 preservation checks
passed, including environment, master key, account settings/credentials,
Cloudflare settings/secrets, transport/profile policies and existing ownership.
Of 4,968 pre-existing profile identities, 4,952 were unchanged and 16 had
independently confirmed normal expiry retirements; 12 new identities appeared.
All 77 pre-existing Trojan identities were unchanged. No unexpected identity
change was found. These are aggregate results; no secret values are exported.

The first production attempt stopped before service replacement because the
free-space recovery reserve was insufficient. The reserve was not weakened.
Six inactive historical runtimes were archived, every file and symlink was
verified against the archive and rechecked before removal, and three immediate
rollback copies were retained. The verified archive hash is recorded in the
evidence JSON. The exact retained candidate was reviewed against the clean
source, binary manifest, static assets and migrations, then passed the original
locked deployment, integrity, backup, space, compatibility and readiness guards.
The original failed-attempt evidence and successful retry evidence are retained.

The public HTTPS panel returned the exact new revision and all five readiness
checks at 2026-10-24T05:51:33Z. Its AWS adapter, app JavaScript and CSS matched
the published source byte for byte. Both integrity/backup timers are active
and enabled, and neither instance has a pending-deployment marker. Separate
pre-upgrade backups have valid pg_restore catalogs; the production update also
created and verified its normal sealed/encrypted backup.

A fresh read-only count still found zero saved AWS accounts. Live customer
pricing permissions, account-specific capacity and EC2 create/delete remain
unverified; no synthetic result is presented as a live account result.

Final API accounting is unchanged: 0 new paid OpenAI calls, 0 additional
reported tokens, USD 0 additional cost. Prior ledger: 3 calls / 6,809 tokens /
USD 0.05716284695 estimated. The three-call cap was respected.
See AWS-UNIFIED-20261024-evidence.json for exact checks and evidence hashes.
