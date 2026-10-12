#!/usr/bin/env bash
set -euo pipefail
export PATH=/usr/local/go/bin:$PATH
: "${BULK_TEST_DATABASE_URL:?isolated test database required}"
mkdir -p .local/panel-proof-headroom
exec > >(tee .local/panel-proof-headroom/acceptance.log) 2>&1
unset DOB_E2E_DSN
export DATABASE_URL="$BULK_TEST_DATABASE_URL" GOMAXPROCS=3
python3 - <<'SNAP'
import hashlib,json,os,pathlib,runpy,subprocess,urllib.parse
assert 'bulk_test' in urllib.parse.urlparse(os.environ['DATABASE_URL']).path
b=pathlib.Path('.local/panel-proof-headroom');s=runpy.run_path('tools/panel-proof-headroom-review.py')['source_snapshot']()
old=pathlib.Path('/root/backups/dob-panel-native-contract-20261012')
prior=json.loads((old/'accepted-tests.json').read_text());review=json.loads((old/'api-review.json').read_text())
assert prior['passed'] and prior['source']==review['source'] and review['verdict']['decision']=='PASS'
h=hashlib.sha256();reused=[]
for p in prior['source']['paths']:
 data=subprocess.check_output(['git','show',s['baseline']+':'+p]);h.update(p.encode()+b'\0'+data+b'\0')
 if p not in s['paths']:
  assert pathlib.Path(p).read_bytes()==data,p
  reused.append(p)
assert h.hexdigest()==prior['source']['sha256']
(b/'test-source.json').write_text(json.dumps(s))
(b/'inherited-tests.json').write_text(json.dumps({'source':prior['source'],'unchanged_paths':reused,'gates':prior['gates'],'inherited':prior.get('inherited'),'limitation':'Prior tests cover only unchanged bytes; new admission behavior receives fresh race and contention tests.'},indent=2))
print('Inherited prior tests only for verified unchanged bytes')
SNAP
git diff --check
go test -p 1 -run '^$' ./...
go test -p 1 -race -count=1 ./cmd/worker ./internal/worker ./internal/panels/usercapacity ./internal/panels/globalreality ./internal/panels/residentialsync
go test -p 1 -race -count=1 -v ./internal/adminapi -run 'TestRouting(SharedQueue|ProofLanes)'
python3 - <<'ACCEPT'
import runpy,json,pathlib,time
b=pathlib.Path('.local/panel-proof-headroom');s=runpy.run_path('tools/panel-proof-headroom-review.py')['source_snapshot']()
assert s==json.loads((b/'test-source.json').read_text()),'source changed during tests'
(b/'accepted-tests.json').write_text(json.dumps({'source':s,'passed':True,'completed_at':time.time(),'gates':['all_packages_compile','race_worker_globalreality_capacity_routing_full_suites','race_postgres_shared_queue_duplicate_and_retirement_fences','bounded_poll_class_routes_admitted_under_bulk_saturation_cancellation_no_leaks','git_diff_check'],'inherited':json.loads((b/'inherited-tests.json').read_text())},indent=2))
ACCEPT
