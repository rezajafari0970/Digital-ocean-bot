#!/usr/bin/env bash
set -euo pipefail
export PATH=/usr/local/go/bin:$PATH
: "${BULK_TEST_DATABASE_URL:?isolated test database required}"
python3 - <<'PYCHECK'
import os,urllib.parse
assert 'bulk_test' in urllib.parse.urlparse(os.environ['BULK_TEST_DATABASE_URL']).path,'isolated database required'
PYCHECK
mkdir -p .local/guardian-reconcile
exec > >(tee .local/guardian-reconcile/acceptance.log) 2>&1
unset DOB_E2E_DSN
export DATABASE_URL="$BULK_TEST_DATABASE_URL" GOMAXPROCS=4
python3 - <<'PYSOURCE'
import runpy,json,pathlib
m=runpy.run_path('tools/guardian-reconcile-review.py')
pathlib.Path('.local/guardian-reconcile/test-source.json').write_text(json.dumps(m['source_snapshot']()))
PYSOURCE
go test -p 2 -count=1 ./...
go test -p 2 -race -count=1 ./internal/serverprotection ./internal/provisioning
bash tools/account-panel-release-test.sh
node --check web/static/app.js
git diff --check
python3 - <<'PYEVIDENCE'
import runpy,json,pathlib,time
m=runpy.run_path('tools/guardian-reconcile-review.py');source=m['source_snapshot']();base=pathlib.Path('.local/guardian-reconcile')
assert source==json.loads((base/'test-source.json').read_text()),'source changed during tests'
(base/'accepted-tests.json').write_text(json.dumps({'source':source,'passed':True,'completed_at':time.time(),'gates':['full_go_suite_isolated_postgres','race_serverprotection_provisioning_with_executed_shell_transitions','real_cross_build_both_guardians_missing_hash_arch_faults','javascript_syntax','git_diff_check']},indent=2))
PYEVIDENCE
