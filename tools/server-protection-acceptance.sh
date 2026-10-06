#!/usr/bin/env bash
set -eo pipefail
cd "$(dirname "$0")/.."
export PATH=/usr/local/go/bin:/usr/sbin:/usr/bin:/sbin:/bin
if [ -z "$GOTMPDIR" ]; then export GOTMPDIR=/root/buildtmp/dob-server-protection; fi
mkdir -p "$GOTMPDIR" .local/protection-ui
if [ -z "$BULK_TEST_DATABASE_URL" ]; then
 set +x
 set -a
 source /etc/digital-ocean-bot/env
 set +a
 BULK_TEST_DATABASE_URL="$(python3 - <<'PY'
import os,urllib.parse
u=urllib.parse.urlsplit(os.environ["DATABASE_URL"])
print(urllib.parse.urlunsplit(u._replace(path="/dob_bulk_test_20261003")))
PY
)"
 export BULK_TEST_DATABASE_URL
fi
python3 - <<'PY'
import os,urllib.parse
u=urllib.parse.urlsplit(os.environ["BULK_TEST_DATABASE_URL"])
assert "bulk_test" in u.path, "isolated test database required"
PY
node --check web/static/server_protection.js
node --check web/static/app.js
git diff --check
env -u BULK_TEST_DATABASE_URL go test -p 2 ./...
DOB_RUN_NFT_TEST=1 go test -race ./internal/serverprotection -count=1 -v
go test -race ./internal/provisioning -run TestSSHGuard -count=1 -v
DOB_RUN_BROWSER_TEST=1 DOB_UI_ARTIFACT_DIR="$PWD/.local/protection-ui" go test -race ./internal/adminapi -run 'TestServerProtection|TestClassOutputDisjointImmutableAndFresh' -count=1 -v
echo SERVER_PROTECTION_ACCEPTANCE_PASS
