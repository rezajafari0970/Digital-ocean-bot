#!/usr/bin/env bash
set -euo pipefail
set +x
cd "$(dirname "$0")/.."
export PATH=/usr/local/go/bin:/usr/sbin:/usr/bin:/sbin:/bin
export GOTMPDIR=/root/buildtmp/dob-hardening
mkdir -p "$GOTMPDIR"
if [ -z "${BULK_TEST_DATABASE_URL:-}" ]; then
 set -a
 source /etc/digital-ocean-bot/env
 set +a
 BULK_TEST_DATABASE_URL="$(python3 - <<'PY'
import os,urllib.parse
u=urllib.parse.urlsplit(os.environ['DATABASE_URL'])
print(urllib.parse.urlunsplit(u._replace(path='/dob_bulk_test_20261003')))
PY
)"
 export BULK_TEST_DATABASE_URL
fi
python3 - <<'PY'
import os,urllib.parse
assert 'bulk_test' in urllib.parse.urlsplit(os.environ['BULK_TEST_DATABASE_URL']).path
PY
env -u BULK_TEST_DATABASE_URL -u DOB_E2E_DSN go test -p 2 ./...
go test -race -p 1 ./internal/app ./internal/adminapi ./internal/worker ./internal/serverprotection ./internal/provisioning ./internal/providers/vultr ./internal/workflow ./internal/scheduler ./internal/panels/clientops -run 'TestInitialSSH|TestPrepared|TestDispatcher|TestServerProtection|TestDashboard|TestAccountBuild|TestAccountDeletion|TestAccountRuleApplication|TestInstallerBootstrap|TestSSHRetry|TestMutationContracts|TestSSHKey|TestProcessFD|TestXrayHealth|TestDrain|TestPostgresRunLease' -count=1 -v
env -u BULK_TEST_DATABASE_URL -u DOB_E2E_DSN DOB_RUN_NFT_TEST=1 go test -race ./internal/serverprotection -count=1 -v
git diff --check
echo FLEET_HARDENING_ACCEPTANCE_PASS
