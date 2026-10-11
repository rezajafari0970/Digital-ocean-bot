#!/usr/bin/env bash
set -euo pipefail
export PATH=/usr/local/go/bin:$PATH
: "${BULK_TEST_DATABASE_URL:?isolated test database required}"
python3 - <<'CHECK_DB'
import os,urllib.parse
if 'bulk_test' not in urllib.parse.urlparse(os.environ['BULK_TEST_DATABASE_URL']).path:raise SystemExit('isolated test database required')
CHECK_DB
mkdir -p .local/panel-audit
exec > >(tee .local/panel-audit/acceptance.log) 2>&1
unset DOB_E2E_DSN
export DATABASE_URL="$BULK_TEST_DATABASE_URL" GOMAXPROCS=3 DOB_PURGE_SCALE=1
python3 - <<'SNAPSHOT'
import runpy,json,pathlib
s=runpy.run_path('tools/panel-audit-review.py')['source_snapshot']()
pathlib.Path('.local/panel-audit/test-source.json').write_text(json.dumps(s))
SNAPSHOT
node --check web/static/app.js
node --check web/static/residential_performance.js
node --check web/tests/config-refresh-recovery.cjs
git diff --check
go test -p 1 -count=1 ./...
go test -p 1 -race -count=1 ./internal/adminapi ./internal/panels/clientops ./internal/panels/usercapacity ./internal/panels/sanaei ./internal/panels/residentialsync ./internal/residentialperf
node web/tests/config-refresh-recovery.cjs
DOB_RUN_BROWSER_TEST=1 go test -p 1 -count=1 -v ./internal/adminapi -run 'Test(AccountDeletionBrowser|BlockedAccountPurgeBrowser)$'
for core in /usr/local/bin/xray /root/backups/dob-residential-allowlist-20261008/xray-v26.9.9; do
 test -x "$core"
 "$core" version
 XRAY_TEST_BINARY="$core" go test -p 1 -count=1 -v ./internal/panels/residentialsync -run '^TestInstalledCore'
done
python3 - <<'ACCEPT'
import runpy,json,pathlib,time
s=runpy.run_path('tools/panel-audit-review.py')['source_snapshot']();b=pathlib.Path('.local/panel-audit')
assert s==json.loads((b/'test-source.json').read_text()),'source changed during tests'
(b/'accepted-tests.json').write_text(json.dumps({'source':s,'passed':True,'completed_at':time.time(),'gates':['full_go_suite_isolated_postgres','race_adminapi_clientops_usercapacity_sanaei_residentialsync_residentialperf','real_chromium_refresh_navigation_failure_timeout_mobile_desktop','real_chromium_account_deletion_regression','installed_xray_26_3_27_and_26_9_9_traffic_policy_failover_restore','javascript_syntax','git_diff_check']},indent=2))
ACCEPT
