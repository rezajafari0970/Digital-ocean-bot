#!/usr/bin/env python3
import json,os,subprocess,sys,re
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT)
def J(p): return json.load(open(p,encoding='utf-8'))
def active(s): return subprocess.run(['systemctl','is-active',s],capture_output=True,text=True).stdout.strip()=='active'
checks=[]
def C(cat,name,ok): checks.append({'n':len(checks)+1,'category':cat,'name':name,'pass':bool(ok)})
S=J('docs/FULL_SOURCE_KNOWLEDGE.json'); M=J('docs/SEMANTIC_KNOWLEDGE.json'); Y=J('docs/SYMBOL_INDEX.json'); DB=J('docs/SCHEMA_INDEX.json'); LDB=J('docs/LIVE_DB_BRAIN.json'); T=J('docs/TEST_MAP.json'); F=J('docs/FRONTEND_BRAIN.json'); L=J('docs/LIFECYCLE_STATE_BRAIN.json'); R=J('docs/INTENT_REQUIREMENTS_BRAIN.json'); I=J('docs/IMPACT_GRAPH.json'); X=J('docs/TRACEABILITY_MATRIX.json'); RW=J('docs/RUNTIME_WORKER_BRAIN.json')
# 1-15 Source/line
for p in ['internal/adminapi/server.go','internal/adminapi/output.go','internal/providers/digitalocean/client.go','internal/providers/vultr/client.go','internal/providers/vultrconsole/browser.go','internal/panels/sanaei/api.go','internal/droplets/lifecycle_engine.go','internal/network/client.go','cmd/api/main.go','cmd/worker/main.go','web/static/app.js']:
 C('source',p,any(f['path']==p for f in S['files']))
C('source','>=600 indexed files',S['file_count']>=600); C('source','>=30000 indexed lines',S['total_lines']>=30000); C('source','>=1500 semantic symbols',M['symbol_count']>=1500); C('source','server.go line 105 retrievable',next(f for f in S['files'] if f['path']=='internal/adminapi/server.go')['line_count']>=105)
# 16-25 Architecture/intent
for rid in ['ARCH-001','ACC-001','NET-001','LIFE-001','DO-001','VULTR-001','VULTR-002','VULTR-003','OUT-001','OPS-001']:
 C('intent',rid,any(x['id']==rid for x in R['requirements']))
# 26-35 Routes/API
for path in ['/api/v1/accounts','/api/v1/output','/api/v1/output/share','/api/v1/providers','/api/v1/configs','/api/v1/config-capacity','/api/v1/proxies','/api/v1/residential-proxies','/api/v1/accounts/{id}/console-session','/version']:
 C('api',path,any(x['path']==path for x in Y['routes']))
# 36-45 DB
C('db','source migrations >=102',DB['migration_count']>=102); C('db','live migrations >=102',LDB['migration_count']>=102); C('db','live tables >=50',LDB['table_count']>=50)
for t in ['accounts','deployments','panel_instances','output_config_snapshots','provider_capacity_observations','schema_migrations','network_profiles']:
 C('db','table '+t,any(x['name']==t for x in LDB['tables']))
# 46-55 Frontend
C('ui','pages >=7',len(F['pages'])>=7); C('ui','api paths >=20',len(F['api_paths'])>=20); C('ui','actions >=20',len(F['actions'])>=20)
for p in ['dashboard','accounts','proxies','residential','configs','output','settings']:
 C('ui','page '+p,any(x['id']==p for x in F['pages']))
# 56-65 Lifecycle/provisioning
C('lifecycle','states >=40',len(L['states'])>=40); C('lifecycle','transition evidence >=50',len(L['transition_evidence'])>=50); C('lifecycle','runtime events >=20',len(RW['runtime_events'])>=20)
for st in ['CREATING','FAILED','DELETED','INSTALLING','INSTALL_COMPLETE','INSTALL_FAILED','CONFIGURING_PANEL']:
 C('lifecycle','state '+st,st in L['states'])
# 66-75 Tests
C('tests','test files >=120',T['test_file_count']>=120); C('tests','test funcs >=250',T['test_function_count']>=250)
for f in ['digitalocean','vultr','network_proxy','compute_lifecycle','sanaei','reality_policy','output','adminapi']:
 C('tests','feature '+f,f in T['features'] and len(T['features'][f]['files'])>0)
# 76-85 Runtime/production
C('runtime','api active',active('digital-ocean-bot-api')); C('runtime','worker active',active('digital-ocean-bot-worker'))
for p in ['tools/verify-production-revision.py','docs/PRODUCTION_REVISION_IDENTITY.md','docs/RUNTIME_MAP.json','docs/RUNTIME_WORKER_BRAIN.json','deploy/upgrade.sh','deploy/healthcheck.sh','internal/buildinfo/buildinfo.go','tools/build-runtime-map.py']:
 C('runtime',p,os.path.exists(p))
# 86-95 impact/traceability
C('impact','nodes >=2000',I['node_count']>=2000); C('impact','edges >=2000',I['edge_count']>=2000); C('impact','trace rows cover requirements',X.get('summary',{}).get('requirements',len(X.get('rows',[])))>=len(R['requirements']))
for rid in ['ARCH-001','NET-001','LIFE-001','VULTR-002','OUT-002','REAL-001','RES-001']:
 C('impact','trace '+rid,any(x['requirement_id']==rid for x in X['rows']))
# 96-100 handoff/drift/retrieval
for p in ['docs/SESSION_HANDOFF_BUNDLE.md','docs/HANDOFF_READINESS_AUDIT.md','tools/project-brain-query.py','tools/source-lookup.py','tools/check-project-continuity.sh']:
 C('handoff',p,os.path.exists(p) and os.path.getsize(p)>0)
assert len(checks)==100,len(checks)
passed=sum(x['pass'] for x in checks); report={'score':passed,'total':100,'ready':passed==100,'checks':checks}
json.dump(report,open('docs/HANDOFF_ACCEPTANCE_100.json','w'),indent=2);open('docs/HANDOFF_ACCEPTANCE_100.json','a').write('\n')
with open('docs/HANDOFF_ACCEPTANCE_100.md','w') as f:
 f.write(f'# Fresh-session Acceptance\n\nScore: **{passed}/100** — **{"READY" if passed==100 else "NOT READY"}**\n\n')
 for x in checks:f.write(f'- [{"x" if x["pass"] else " "}] {x["n"]:03d} `{x["category"]}` — {x["name"]}\n')
print(f'ACCEPTANCE {passed}/100 '+('READY' if passed==100 else 'NOT READY'))
for x in checks:
 if not x['pass']: print(f'FAIL {x["n"]:03d} [{x["category"]}] {x["name"]}')
sys.exit(0 if passed==100 else 1)
