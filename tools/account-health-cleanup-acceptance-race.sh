#!/usr/bin/env bash
set -euo pipefail
export PATH=/usr/local/go/bin:$PATH
: "${BULK_TEST_DATABASE_URL:?isolated test database required}"
python3 - <<'PY'
import os,urllib.parse
if 'bulk_test' not in urllib.parse.urlparse(os.environ['BULK_TEST_DATABASE_URL']).path:raise SystemExit('isolated test database required')
PY
mkdir -p .local/account-health-cleanup
exec > >(tee .local/account-health-cleanup/acceptance.log) 2>&1
unset DOB_E2E_DSN
export DATABASE_URL="$BULK_TEST_DATABASE_URL" GOMAXPROCS=3 DOB_PURGE_SCALE=1
python3 - <<'PY'
import runpy,json,pathlib
s=runpy.run_path('tools/account-health-cleanup-review.py')['source_snapshot']()
pathlib.Path('.local/account-health-cleanup/test-source.json').write_text(json.dumps(s))
PY
go test -p 1 -count=1 ./...
go test -p 1 -race -count=1 ./internal/app ./internal/adminapi ./internal/providers/... ./internal/capacity ./internal/worker
DOB_RUN_BROWSER_TEST=1 go test -p 1 -count=1 -v ./internal/adminapi -run 'Test(AccountDeletionBrowser|BlockedAccountPurgeBrowser)$'
node --check web/static/app.js
git diff --check
python3 - <<'PY'
import runpy,json,pathlib,time
s=runpy.run_path('tools/account-health-cleanup-review.py')['source_snapshot']();b=pathlib.Path('.local/account-health-cleanup')
assert s==json.loads((b/'test-source.json').read_text()),'source changed during tests'
(b/'accepted-tests.json').write_text(json.dumps({'source':s,'passed':True,'completed_at':time.time(),'gates':['full_go_suite_isolated_postgres','race_app_adminapi_all_providers_capacity_worker','large_history_purge_435_panels_970000_sync_rows','real_chromium_deletion_durable_queue_lost_response_restart_worker_native_fence_health_details_mobile_desktop','javascript_syntax','git_diff_check']},indent=2))
PY
