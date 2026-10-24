# AWS catalog repair — 2026-10-24

Completed verification at 04:35 UTC / 08:05 Asia/Tehran.

Public release: 27257387077ff80e2e2a4d7539bc61d3bb7a2e06.
Both independent instances run this exact revision. The existing public
85.192.63.181:8443 gateway returns the new version and all five readiness checks.

## Defect and correction

The AWS image filter required exactly one block-device mapping. EC2 permits
a single EBS root together with optional instance-store mappings. Consequently
valid Canonical Ubuntu images could disappear from the catalog and fail the
same check at creation.

The driver now distinguishes EBS, virtualName and noDevice mappings. It
accepts optional ephemeral or suppressed non-root mappings while rejecting
additional EBS volumes, duplicate devices/virtual mappings, absent or suppressed
roots, and conflicting mapping types. Canonical ownership, public status,
supported Ubuntu versions, x86_64/HVM architecture and product-code restrictions
remain enforced. Catalog and CreateServer use the same validator.

Empty results produce allowlisted diagnostics for unavailable supported plans,
an empty image response, or images rejected by the compatibility checks.
The guidance refers to the selected region, does not label a successful read
as a missing IAM permission, and does not expose raw provider messages.
AWS catalog cache version is advanced to invalidate earlier cached results.

Reference:
https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_BlockDeviceMapping.html
https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/block-device-mapping-concepts.html

## Evidence

- Overlaying the pre-fix catalog reproduces four valid mapping cases being
  rejected and two conflicting root cases being accepted.
- Seventeen wire-response mapping cases exercise both catalog and creation.
  Four empty-catalog cases and three API diagnostic cases cover read-only
  behavior, actionable errors and the secret boundary.
- Full Go tests/vet, installer Python checks and web checks pass.
- 147 mandatory database/race/native regressions pass, with zero skips.
- Live-secret comparison against both instance configurations and gitleaks pass.
- Exact-candidate fresh install and repeated-install identity checks pass on
  disposable Ubuntu 24.04 GitHub-hosted runners.
- Main and stable were promoted atomically from 17a1ce99 to 27257387.
- CI candidate run: 40061569260; main: 40062432444; stable: 40062432349.
- Both instances: three services active, matching executable hashes, zero
  restarts, schema 000195_provider_snapshot_latest, five readiness checks.
- All 14 policy/credential/identity preservation checks pass on each instance.
  Production compared 4,916 profile identities and 77 Trojan identities;
  eight already-expired retirements were independently confirmed and there
  were zero unexpected changes.

CI:
https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40061569260

## Environment repairs and limits

Source and builds moved to the healthy secondary host after legacy Git
crashed with signal 11. Legacy LXD also failed with an invalid type descriptor.
The old database/application remain stopped and quarantined. Its publisher
now refuses execution when /etc/dob-controller-quarantined exists.
The gateway remains on that host; the underlying host failure is unresolved.

The release gate originally depended on global geosite data from the old
host. Its independent replacement pins and verifies both native core and
geosite data. An initial 1200 MiB race-build cgroup exhausted its limit;
the successful run used bounded 1800 MiB, GOMAXPROCS=1 and GOMEMLIMIT=768MiB.

Low disk space had blocked the default instance's scheduled backup. Seventy-four
old rollback runtimes were archived, every archived file was hash verified,
and five recent rollback copies were kept ready. The full compressed archive
is retained; no application database/configuration was deleted.
Both integrity/backup services then completed successfully and both timers
remain armed. Temporary source-transfer SSH grants/keys were removed.
The unused LXD installation on the secondary was removed.

No saved AWS account was available to replay the customer's live credentials.
The EC2 responses in regression tests are synthetic. The customer must retry
credential validation to confirm this fix against their actual AWS catalog.

## Continuity and usage

Healthy source: /root/projects/dob-aws-catalog-20261024.
Evidence: /root/dob-aws-catalog-20261024.
Exact public candidate/evidence: /var/lib/datacenter-publisher/aws-20261024.
Separate default-rollout and production-rollout directories hold backups,
before/after snapshots and update logs. Do not overwrite prior baselines.

This stage used zero new OpenAI API calls, tokens or dollars.
Recorded cumulative usage remains 3 requests, 6,809 tokens, estimated
USD 0.05716284695. The three-request cap is exhausted; no additional paid call
was made. The USD 0.50 budget was not increased.
