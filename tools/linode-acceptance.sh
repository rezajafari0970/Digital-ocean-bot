#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
test "${1:-}" = "-race"
python3 - <<'PY'
import os,urllib.parse
u=urllib.parse.urlsplit(os.environ.get('BULK_TEST_DATABASE_URL',''))
assert 'bulk_test' in u.path and u.scheme in ('postgres','postgresql'),'isolated bulk_test database required'
PY
mkdir -p .local/linode
git diff --check
node --check web/static/app.js
node --check web/tests/linode.cjs
python3 - <<'PY'
import json,runpy,pathlib
m=runpy.run_path('tools/linode-review.py')
pathlib.Path('.local/linode/test-source.json').write_text(json.dumps(m['source_snapshot'](),indent=2))
PY
go test -race -p 1 -count=1 -json -timeout 8m ./internal/providers/... > .local/linode/provider-tests.jsonl
export DOB_RUN_BROWSER_TEST=1
go test -race -p 1 -count=1 -json -timeout 10m ./internal/app ./internal/adminapi ./internal/droplets ./internal/capacity ./internal/workflow -run 'Test(Linode|MutationFence|AccountBuildParity|AccountDeletion|AccountRuleApplication|UpCloudBrowser|Executor|Capacity|Create|Delete|Operation)' > .local/linode/integration-tests.jsonl
env -u BULK_TEST_DATABASE_URL -u DOB_RUN_BROWSER_TEST go test -p 2 -count=1 -json -timeout 8m ./... > .local/linode/repository-tests.jsonl
python3 - <<'PY'
import json,pathlib,runpy,datetime
b=pathlib.Path('.local/linode');summary={}
for name in ['provider','integration','repository']:
 events=[json.loads(s) for s in (b/(name+'-tests.jsonl')).read_text().splitlines() if s.startswith('{')]
 passed=[e for e in events if e.get('Test') and e['Action']=='pass']
 failed=[e for e in events if e['Action']=='fail']
 skipped=[e for e in events if e.get('Test') and e['Action']=='skip']
 assert not failed,(name,failed)
 if name!='repository':assert not skipped,(name,'required gate skipped',skipped)
 summary[name]={'passed':len(passed),'main_tests':sum('/' not in e['Test'] for e in passed),'skipped':len(skipped),'failed':len(failed)}
 if name=='repository':
  (b/'repository-opt-in-skips.json').write_text(json.dumps([{'package':e['Package'],'test':e['Test']} for e in skipped],indent=2))
source=runpy.run_path('tools/linode-review.py')['source_snapshot']()
assert source==json.loads((b/'test-source.json').read_text()),'source changed during acceptance'
evidence={'passed':True,'source':source,'completed_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'tests':summary,'live_linode_acceptance':False,'production_database_used':False}
(b/'accepted-tests.json').write_text(json.dumps(evidence,indent=2)+'\n')
(b/'acceptance.log').write_text(json.dumps(evidence,indent=2)+'\n')
print(json.dumps(evidence,indent=2))
PY
