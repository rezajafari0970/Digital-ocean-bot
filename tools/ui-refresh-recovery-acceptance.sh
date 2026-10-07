#!/usr/bin/env bash
set -euo pipefail
set +x
cd "$(dirname "$0")/.."
export PATH=/usr/local/go/bin:/usr/sbin:/usr/bin:/sbin:/bin
export GOTMPDIR=/root/buildtmp/dob-client-isolation
export DOB_UI_ARTIFACT_DIR=/root/backups/dob-ui-refresh-recovery-20261007/browser
mkdir -p "$GOTMPDIR"
node --check web/static/app.js
node web/tests/config-refresh-recovery.cjs
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
python3 - <<'PY'
import os,urllib.parse
assert 'bulk_test' in urllib.parse.urlsplit(os.environ['BULK_TEST_DATABASE_URL']).path
PY
go test -race ./internal/adminapi -run 'Test.*(Capacity|Recovery|Output|Admission|Isolation)' -count=1 -v
env -u BULK_TEST_DATABASE_URL -u DOB_E2E_DSN go test -p 2 ./...
git diff --check
echo UI_REFRESH_RECOVERY_ACCEPTANCE_PASS
