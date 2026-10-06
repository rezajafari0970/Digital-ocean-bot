#!/usr/bin/env bash
set -euo pipefail
set +x
cd "$(dirname "$0")/.."
export PATH=/usr/local/go/bin:/usr/sbin:/usr/bin:/sbin:/bin
export GOTMPDIR=/root/buildtmp/dob-delete-resume
mkdir -p "$GOTMPDIR"
if [ -z "$(printenv BULK_TEST_DATABASE_URL || true)" ];then
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
go test -race ./internal/droplets -run 'TestDelete' -count=1 -v
git diff --check
echo DELETE_RESUME_ACCEPTANCE_PASS
