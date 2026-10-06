#!/usr/bin/env bash
set -eo pipefail
cd "$(dirname "$0")/.."
export PATH=/usr/local/go/bin:/usr/sbin:/usr/bin:/sbin:/bin
if [ -z "$GOTMPDIR" ]; then export GOTMPDIR=/root/buildtmp/dob-account-rule; fi
mkdir -p "$GOTMPDIR" .local/account-rule-ui
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
node --check web/static/app.js
git diff --check
if [ "$1" = "full" ] || [ "$1" = "-race" ]; then env -u BULK_TEST_DATABASE_URL go test -p 2 ./...; fi
DOB_RUN_BROWSER_TEST=1 DOB_UI_ARTIFACT_DIR="$PWD/.local/account-rule-ui" go test -race ./internal/adminapi -run 'TestAccountRuleApplication' -count=1 -v
echo ACCOUNT_RULE_APPLICATION_ACCEPTANCE_PASS
