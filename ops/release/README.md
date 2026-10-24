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
