> Latest continuation: [cleanup statistics and provider diagnostics, November 6](../../docs/CLEANUP_DIAGNOSTICS_RELEASE_20261106.md). Both instances accepted at the recorded revision; no rollout remains pending.

> Latest continuation: [November 3 live rollout](../../docs/PROVIDER_BROWSER_ROLLOUT_20261103.md). [Authorized cleanup is complete](../../docs/STORAGE_CLEANUP_20261031.md). [Verified release and rollout instructions](../../docs/BUILD_ACTIVITY_CONTINUATION_20261030.md). Older entries below retain their dates.

# Independent release verification

The legacy controller is quarantined. On 2026-10-24 Git worktree creation
crashed with signal 11 and LXD forkstart failed with an invalid Go type
descriptor. These are host failures, not evidence about the AWS patch.
Do not build releases or run the application's database on that host.

Public candidates use release-candidate/* branches. Source checks and an
Ubuntu 24.04 installation job must both succeed for the exact candidate SHA.
The installation job starts from a restrictive root checkout, checks login,
static assets and all five readiness checks, and repeats installation to
verify retained credentials and identity. The smoke helper refuses execution
outside a disposable GitHub-hosted runner.

The single gate implementation is tools/public-release/transport-regression-gate.py.
The ops entry point delegates to it; the private publisher can still copy that
self-contained file into its credential-free sandbox. Entrypoint/copy parity and
fail-closed evidence tests are mandatory in fleet-verification. The 2026-10-25
continuation repaired a 43-test policy drift between the former implementations;
both paths now require the same 192 tests, pinned native assets and browser gates.
Installed publishers must be updated before relying on this new policy.

Before promotion, run transport-regression-gate.py on an independent GitHub-hosted
runner with a disposable private-socket PostgreSQL and no production database
variables. It requires every named test and subtest to pass without skips.
The native core, Chrome and geosite data are pinned and hash checked. The private
fleet-verification workflow checks the exact PR head, full Go tests/vet, mobile
browser flows, native transport and race/ownership regressions.

Do not run heavy native/race tests on the 4 GiB production recovery host. A 2 GiB
test cgroup still caused global memory pressure on 2026-10-24; both worker role
checks failed closed and systemd recovered them. For ordinary deployment builds,
use a separate bounded service (MemoryMax=1G, MemoryHigh=768M, GOMEMLIMIT=384MiB,
GOFLAGS=-p=1, CPUQuota=100%) and verify live worker health throughout. Never remove
ownership guards or lower backup disk requirements to make verification pass.

Run the live-secret scanner against both independent instance configurations
and gitleaks against the exact export. Verify release/FILES.json and a clean
Git tree. Promote main and stable atomically only after all gates pass.
Keep the prior stable SHA as the expected destination and do not overwrite
independent remote changes.

Production uses /usr/local/sbin/dob-production update; the default independent
instance uses /usr/local/bin/datacenter-bot update. Capture separate baselines
and verify revision, executable hashes, policies, credentials and readiness
after each. Never start the quarantined legacy PostgreSQL cluster.

2026-10-24 AWS candidate: 27257387077ff80e2e2a4d7539bc61d3bb7a2e06.
Independent install/test evidence:
https://github.com/rezajafari0970/Datacenter-create-by-api-and-ssh-and-install-x-ui-and-create-config/actions/runs/40061569260

No saved AWS account was available for a customer-account replay. The mapping
regression uses synthetic EC2 wire responses, not a claimed live AWS result.

## GitHub-hosted atomic promotion, 2026-10-24

The AWS unified candidate is 272c46317edff036c32e302c8adce4f0ff3dcda7.
Source and independent install run 40066053925 passed; atomic promotion run
40066721562 passed. The operational workflow is preserved in
`promote-verified-aws-20261024.yml`. It ran on a temporary operational branch,
not as an additional workflow in the released application.

This path needs no publication process on the quarantined host. It verifies
the exact candidate SHA, successful test/install jobs, release file manifest
and baseline ancestry, then pushes main and stable in one atomic operation
with leases on both expected old heads. The temporary GitHub workflow token
is confined to the job. A destination change aborts promotion.

For a future release, create a new operational branch and update all pinned
SHAs and the verified run only after its independent evidence passes. Do not
rerun this historical workflow as a generic latest-release publisher. Secret scanning, database, native and race gates remain prerequisites; hosted
source and installation jobs supplement those checks. The old legacy publisher
state must not be treated as current while that host is quarantined.

## Latest verified continuation — 2026-10-25

See `CDN-PROOF-ISOLATION-20261025.md` for the current release, exact CI evidence, live rollout status, rejected AWS credentials and DNS capacity constraints. Do not treat older saved AWS/account observations as current.

Post-release continuation: `POSTRELEASE-OBSERVATION-20261025.md` records three independently completed new builds, current health and the still-unresolved external blockers at 06:17 Tehran.

Latest completed release: `FLEET-DNS-CAPACITY-20261025.md` records live release `5dc2c25678698dbbe9ac1cfee7d18701c19213e7`, 187 passing mandatory checks, capacity forecasting and the remaining AWS/DNS/gateway blockers.

For future operator updates, invoke `ops/release/run-bounded-update.sh /usr/local/sbin/dob-production update` or `ops/release/run-bounded-update.sh /usr/local/bin/datacenter-bot update` from the root-owned source checkout. The launcher was verified against actual kernel cgroup limits. Direct invocation from a remote-command service does not provide a dedicated build limit. Preserve the existing baseline, exact-revision, ownership and backup checks around either command.

## Account diagnostics and recovery — 2026-11-05

See [the account audit](../../docs/ACCOUNT_DIAGNOSTICS_20261105.md) and
`ACCOUNT_DIAGNOSTICS_20261105-evidence.json` for the exact source, public release,
independent CI, guarded rollout, live account results and external blockers.
The canonical gate now requires 359 roots with zero skips. Source PR 37
includes shared list/detail diagnostics for all seven providers and the
terminal-evidence pruning fix found by repeated recovery validation.
The pinned operational workflow is
`promote-verified-account-diagnostics-20261105.yml`; do not rerun it for a future release.

PR 38 fixes the populated account dashboard overflow discovered during live
acceptance. Final public revision `a3ca806e984aaf0c24aad0e89befe9b31738d03a`
passed the same complete independent gate, fresh install and reinstall, then
all guarded rollout stages on both instances. Genuine served pages passed six
physical viewport widths under normal production CSP. The pinned final promotion
is `promote-verified-account-layout-20261105.yml`; it is also historical evidence,
not a reusable latest-release publisher. Provider locks, rejected credentials and
four unknown creates remain explicit external blockers; AWS and UpCloud have no
saved live accounts. Port 18443 and all listener ownership checks were preserved.
