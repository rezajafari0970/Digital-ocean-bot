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

Before promotion, also run transport-regression-gate.py as an unprivileged
user with a disposable private-socket PostgreSQL and no production database
variables. It requires every named test and subtest to pass without skips.
The native core and geosite data are pinned and hash checked; no global
controller geodata installation is required. Use a bounded service and
GOMAXPROCS=1/GOMEMLIMIT=768MiB on the 4 GiB recovery host; the earlier 1200 MiB
cgroup limit was insufficient for cold race compilation.

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
rerun this historical workflow as a generic latest-release publisher. Local
secret, database, native and race gates remain prerequisites; hosted source
and installation jobs supplement those checks. The old legacy publisher
state must not be treated as current while that host is quarantined.
