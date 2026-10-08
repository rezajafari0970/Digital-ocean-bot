#!/usr/bin/env bash
set -euo pipefail
export PATH=/usr/local/go/bin:$PATH
: "${BULK_TEST_DATABASE_URL:?isolated test database required}"
python3 - <<'PY'
import os,urllib.parse
u=urllib.parse.urlparse(os.environ['BULK_TEST_DATABASE_URL'])
assert 'bulk_test' in u.path,'isolated database required'
PY
mkdir -p .local/proxy-economy
exec > >(tee .local/proxy-economy/acceptance.log) 2>&1
unset DOB_E2E_DSN
export DATABASE_URL="$BULK_TEST_DATABASE_URL"
python3 - <<'PYCODE'
import runpy,json,pathlib
m=runpy.run_path('tools/proxy-economy-review.py')
pathlib.Path('.local/proxy-economy/test-source.json').write_text(json.dumps(m['source_snapshot']()))
PYCODE
GOMAXPROCS=4 go test -p 2 -count=1 ./...
GOMAXPROCS=4 go test -p 2 -race -count=1 ./internal/network ./internal/app ./internal/proxycontrol ./internal/providers/digitalocean ./internal/panels/sanaei ./internal/adminapi ./internal/worker
git diff --check

python3 - <<'PYCODE'
import runpy,json,pathlib,time
m=runpy.run_path('tools/proxy-economy-review.py');source=m['source_snapshot']()
assert source==json.loads(pathlib.Path('.local/proxy-economy/test-source.json').read_text()),'source changed during tests'
pathlib.Path('.local/proxy-economy/accepted-tests.json').write_text(json.dumps({'source':source,'passed':True,'completed_at':time.time(),'gates':['full_go_suite_isolated_postgres','race_network_app_proxycontrol_digitalocean_sanaei_adminapi_worker','git_diff_check']},indent=2))
PYCODE
