#!/usr/bin/env bash
set -euo pipefail
export PATH=/usr/local/go/bin:$PATH
: "${BULK_TEST_DATABASE_URL:?isolated test database required}"
python3 - <<'PY'
import os,urllib.parse
if 'bulk_test' not in urllib.parse.urlparse(os.environ['BULK_TEST_DATABASE_URL']).path:raise SystemExit('isolated test database required')
PY
mkdir -p .local/operational-release
exec > >(tee .local/operational-release/acceptance.log) 2>&1
unset DOB_E2E_DSN
export DATABASE_URL="$BULK_TEST_DATABASE_URL" GOMAXPROCS=4
python3 - <<'PY'
import runpy,json,pathlib
s=runpy.run_path('tools/operational-release-review.py')['source_snapshot']()
pathlib.Path('.local/operational-release/test-source.json').write_text(json.dumps(s))
PY
go test -p 1 -count=1 ./...
go test -p 2 -race -count=1 ./internal/app ./internal/adminapi ./internal/network ./internal/serverprotection ./internal/provisioning ./internal/panels/residentialsync
DOB_RUN_BROWSER_TEST=1 go test -p 1 -count=1 -v ./internal/adminapi -run 'Test(AccountDeletionBrowser|BlockedAccountPurgeBrowser)$'
if test -x /usr/local/bin/xray; then
 XRAY_TEST_BINARY=/usr/local/bin/xray go test -p 1 -count=1 ./internal/panels/residentialsync -run TestInstalledCore
fi
bash tools/account-panel-release-test.sh
node --check web/static/app.js
git diff --check
python3 - <<'PY'
import runpy,json,pathlib,time
s=runpy.run_path('tools/operational-release-review.py')['source_snapshot']();b=pathlib.Path('.local/operational-release')
if s!=json.loads((b/'test-source.json').read_text()):raise SystemExit('source changed during tests')
(b/'accepted-tests.json').write_text(json.dumps({'source':s,'passed':True,'completed_at':time.time(),'gates':['full_go_suite_isolated_postgres','race_app_adminapi_network_serverprotection_provisioning_residentialsync','real_chromium_account_deletion_and_blocked_purge','installed_core_allowlist_if_available','real_guardian_cross_build_hash_arch_faults','javascript_syntax','git_diff_check']},indent=2))
PY
