#!/usr/bin/env python3
import json,os,subprocess,sys,collections,re
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT)
def J(p):return json.load(open(p,encoding='utf-8'))
D=J('docs/DEEP_REQUIREMENTS_BRAIN.json'); F=J('docs/FUNCTION_BEHAVIOR_BRAIN.json'); C=J('docs/COLUMN_BEHAVIOR_BRAIN.json'); TB=J('docs/TEST_BEHAVIOR_BRAIN.json'); ST=J('docs/STATE_TRANSITION_BRAIN.json'); UI=J('docs/FRONTEND_ACTION_STATE_BRAIN.json'); IR=J('docs/INCIDENT_REGRESSION_BRAIN.json'); SY=J('docs/SYMBOL_INDEX.json'); SRC=J('docs/FULL_SOURCE_KNOWLEDGE.json'); LIVE=J('docs/LIVE_DB_BRAIN.json'); IMP=J('docs/IMPACT_GRAPH.json'); TR=J('docs/TRACEABILITY_MATRIX.json')
checks=[]
def c(area,name,ok,evidence=''):checks.append({'area':area,'name':name,'pass':bool(ok),'evidence':evidence})
# Intent/architecture: fine-grained requirements + incidents + impact
for area in ['architecture','accounts','digitalocean','vultr','proxy_network','lifecycle','provisioning','sanaei_panel','reality','config_users','output','residential','database','admin_api','frontend','tests','deployment_runtime','continuity']:
 n=sum(x['area']==area for x in D['requirements']);c(area,'deep requirements >=4',n>=4,str(n));c(area,'intent present',n>0)
# Function behavior evidence per major package/domain
pkg_expect={'admin_api':'adminapi','accounts':'accounts','proxy_network':'network','lifecycle':'droplets','provisioning':'provisioning','sanaei_panel':'sanaei','digitalocean':'digitalocean','vultr':'vultr'}
for area,pkg in pkg_expect.items():
 fs=[x for x in F['functions'] if x['package']==pkg];c(area,'function behavior indexed',len(fs)>0,str(len(fs)));c(area,'call relationships present',any(x['calls'] for x in fs));c(area,'error evidence present',any(x['error_return_evidence'] for x in fs))
# DB understanding
c('database','live columns >=500',C['column_count']>=500,str(C['column_count']));c('database','reader evidence >=400',C['statistics']['columns_with_reader_functions']>=400);c('database','writer evidence >=400',C['statistics']['columns_with_writer_functions']>=400);c('database','migration origins >=500',C['statistics']['columns_with_migration_origin']>=500)
for key in [('output_config_snapshots','visible_until'),('accounts','provider'),('schema_migrations','checksum')]:c('database',f'column {key[0]}.{key[1]}',any(x['table']==key[0] and x['column']==key[1] for x in C['columns']))
# Lifecycle depth
c('lifecycle','>=50 states',ST['state_count']>=50,str(ST['state_count']));c('lifecycle','>=40 state-write owners',sum(bool(x['transition_write_evidence']) for x in ST['owners'])>=40);c('lifecycle','>=40 guard owners',sum(bool(x['guard_evidence']) for x in ST['owners'])>=40)
# Frontend depth
c('frontend','>=20 functions',UI['function_count']>=20);c('frontend','>=30 actions',UI['action_count']>=30);c('frontend','API-bound functions >=15',sum(bool(x['api_literals']) for x in UI['functions'])>=15);c('frontend','error evidence >=15',sum(bool(x['error_evidence']) for x in UI['functions'])>=15)
# Tests behavior
c('tests','251 tests indexed',TB['test_function_count']>=251);c('tests','all have assertions',sum(bool(x['assertion_evidence']) for x in TB['tests'])==TB['test_function_count']);c('tests','all have candidate calls',sum(bool(x['candidate_calls']) for x in TB['tests'])==TB['test_function_count'])
for pkg in ['digitalocean','vultr','network','droplets','sanaei','adminapi']:c('tests','package '+pkg,any(x['package']==pkg for x in TB['tests']))
# Incidents/history
for area in ['provisioning','proxy_network','output','providers','vultr','vultr_console','database','production','continuity']:c('history','incident '+area,any(x['area']==area for x in IR['incidents']))
# API/source exactness
c('admin_api','routes >=62',SY['route_count']>=62,str(SY['route_count']));c('source','files >=630',SRC['file_count']>=630,str(SRC['file_count']));c('source','lines >=33000',SRC['total_lines']>=33000,str(SRC['total_lines']))
for p,line in [('internal/adminapi/server.go',105),('web/static/app.js',100),('internal/providers/vultr/read_driver.go',28),('internal/adminapi/output.go',285)]:
 f=next((x for x in SRC['files'] if x['path']==p),None);c('source',f'exact {p}:{line}',bool(f and f['line_count']>=line))
# impact/traceability
c('architecture','impact nodes >=2000',IMP['node_count']>=2000);c('architecture','impact edges >=2000',IMP['edge_count']>=2000);c('architecture','traceability rows >=16',len(TR['rows'])>=16)
# Runtime knowledge artifacts only; no production mutation.
for p in ['docs/RUNTIME_MAP.json','docs/RUNTIME_WORKER_BRAIN.json','docs/PRODUCTION_REVISION_IDENTITY.md','tools/verify-production-revision.py']:c('deployment_runtime',p,os.path.exists(p))
# Scores per area
areas=collections.defaultdict(list)
for x in checks:areas[x['area']].append(x)
scores={a:round(100*sum(x['pass'] for x in xs)/len(xs),1) for a,xs in sorted(areas.items())};passed=sum(x['pass'] for x in checks);total=len(checks)
out={'schema_version':1,'check_count':total,'passed':passed,'overall_percent':round(100*passed/total,2),'area_scores':scores,'checks':checks,'note':'Evidence-backed knowledge mastery exam. 100 means all defined recoverability/understanding evidence gates pass; it does not mean unaided memorization of every source token.'}
json.dump(out,open('docs/DEEP_MASTERY_EXAM.json','w'),ensure_ascii=False,indent=2);open('docs/DEEP_MASTERY_EXAM.json','a').write('\n')
with open('docs/DEEP_MASTERY_EXAM.md','w') as f:
 f.write(f'# Deep Knowledge Mastery Exam\n\nOverall: **{out["overall_percent"]}%** ({passed}/{total})\n\n| Area | Score |\n|---|---:|\n');
 for a,s in scores.items():f.write(f'| `{a}` | **{s}%** |\n')
 f.write('\n## Failed gates\n')
 for x in checks:
  if not x['pass']:f.write(f'- `{x["area"]}` — {x["name"]}'+(f' ({x["evidence"]})' if x['evidence'] else '')+'\n')
print('DEEP MASTERY',out['overall_percent'],f'({passed}/{total})')
for a,s in scores.items():print(f'{a}: {s}%')
for x in checks:
 if not x['pass']:print('FAIL',x['area'],x['name'],x['evidence'])
sys.exit(0)
