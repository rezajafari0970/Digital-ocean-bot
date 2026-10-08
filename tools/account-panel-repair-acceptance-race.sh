#!/usr/bin/env bash
set -euo pipefail
export PATH=/usr/local/go/bin:$PATH
: "${BULK_TEST_DATABASE_URL:?isolated test database required}"
python3 - <<'PYCHECK'
import os,urllib.parse
assert 'bulk_test' in urllib.parse.urlparse(os.environ['BULK_TEST_DATABASE_URL']).path,'isolated database required'
PYCHECK
mkdir -p .local/account-panel-repair
exec > >(tee .local/account-panel-repair/acceptance.log) 2>&1
unset DOB_E2E_DSN
export DATABASE_URL="$BULK_TEST_DATABASE_URL" GOMAXPROCS=4
python3 - <<'PYSOURCE'
import runpy,json,pathlib
m=runpy.run_path('tools/account-panel-repair-review.py')
pathlib.Path('.local/account-panel-repair/test-source.json').write_text(json.dumps(m['source_snapshot']()))
PYSOURCE
go test -p 2 -count=1 ./...
go test -p 2 -race -count=1 ./internal/network ./internal/app ./internal/adminapi ./internal/serverprotection ./internal/providers/digitalocean ./internal/worker
DOB_RUN_BROWSER_TEST=1 go test -p 1 -count=1 -v ./internal/adminapi -run 'Test(AccountDeletionBrowser|BlockedAccountPurgeBrowser)$'
bash tools/account-panel-release-test.sh
node --check web/static/app.js
git diff --check
python3 - <<'PYEVIDENCE'
import runpy,json,pathlib,time
m=runpy.run_path('tools/account-panel-repair-review.py');source=m['source_snapshot']();base=pathlib.Path('.local/account-panel-repair')
assert source==json.loads((base/'test-source.json').read_text()),'source changed during tests'
(base/'accepted-tests.json').write_text(json.dumps({'source':source,'passed':True,'completed_at':time.time(),'gates':['full_go_suite_isolated_postgres','race_network_app_adminapi_serverprotection_digitalocean_worker','real_chromium_deletion_and_blocked_purge','real_cross_build_both_guardians_missing_hash_arch_faults','javascript_syntax','git_diff_check']},indent=2))
PYEVIDENCE
