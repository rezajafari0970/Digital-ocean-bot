#!/usr/bin/env python3
import json,os,subprocess,collections
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
def J(p):return json.load(open(p,encoding='utf-8'))
D=J('docs/DEEP_REQUIREMENTS_BRAIN.json'); F=J('docs/FUNCTION_BEHAVIOR_BRAIN.json'); C=J('docs/COLUMN_BEHAVIOR_BRAIN.json'); T=J('docs/TEST_BEHAVIOR_BRAIN.json'); IR=J('docs/INCIDENT_REGRESSION_BRAIN.json'); SY=J('docs/SYMBOL_INDEX.json')
areas=sorted(set(x['area'] for x in D['requirements'])); aliases={'admin_api':['adminapi'],'sanaei_panel':['sanaei'],'proxy_network':['network'],'digitalocean':['digitalocean'],'vultr':['vultr','vultrconsole'],'lifecycle':['droplets','scheduler','worker'],'provisioning':['provisioning'],'reality':['reality','realityconfig','realitycontract','globalreality'],'accounts':['accounts'],'output':['adminapi','sanaei'],'residential':['residentialsync','adminapi'],'deployment_runtime':['app','worker'],'config_users':['policy','usercapacity','adminapi'],'frontend':[],'database':[],'tests':[],'architecture':['providers','app'],'continuity':[]}
items=[]
for area in areas:
 req=[x for x in D['requirements'] if x['area']==area]; pkgs=aliases.get(area,[]); funcs=[x for x in F['functions'] if x['package'] in pkgs]
 files=sorted(set(x['file'] for x in funcs)); tables=sorted(set(t for x in funcs for t in x.get('db_reads',[])+x.get('db_writes',[])))
 routes=[]
 for r in SY['routes']:
  h=r.get('handler_definition') or {}
  if h.get('file') in files:routes.append({'method':r['method'],'path':r['path'],'handler':r['handler']})
 tests=[x for x in T['tests'] if x['package'] in pkgs]; incidents=[x['id'] for x in IR['incidents'] if x['area']==area or (area=='architecture' and x['area']=='providers') or (area=='deployment_runtime' and x['area']=='production')]
 deps=sorted(set(c for x in funcs for c in x.get('calls',[])))[:200]
 failures=[]
 for x in funcs:
  for e in x.get('error_return_evidence',[]):
   if e not in failures:failures.append(e)
 item={'subsystem':area,'purpose_requirements':[{'id':x['id'],'text':x['requirement']} for x in req],'responsibility_evidence':sorted(set(x.get('structural_responsibility','') for x in funcs if x.get('structural_responsibility'))),'owned_files':files,'function_count':len(funcs),'api_surface':routes,'db_tables':tables,'dependency_call_symbols':deps,'test_count':len(tests),'tests':[{'name':x['test'],'file':x['file'],'line':x['start_line']} for x in tests[:100]],'failure_evidence':failures[:80],'incident_guardrails':incidents}
 items.append(item)
out={'schema_version':1,'source_commit':head,'subsystem_count':len(items),'subsystems':items,'methodology':'composes intent + function behavior + routes + DB touches + tests + incident guardrails into one subsystem view; structural evidence is not a substitute for exact source/runtime'}
json.dump(out,open('docs/SUBSYSTEM_EXPLANATION_BRAIN.json','w'),ensure_ascii=False,indent=2);open('docs/SUBSYSTEM_EXPLANATION_BRAIN.json','a').write('\n')
md=['# Subsystem Deep Explanation Brain','',f'Commit `{head}` — **{len(items)} subsystem knowledge views**.','', 'Each subsystem view composes purpose/requirements, structural responsibilities, owned files/functions, API surface, DB touches, dependencies, tests, failure evidence and historical guardrails.','']
for x in items:
 md += [f'## `{x["subsystem"]}`',f'- Requirements: **{len(x["purpose_requirements"])}**',f'- Functions: **{x["function_count"]}**',f'- API routes: **{len(x["api_surface"])}**',f'- DB tables: **{len(x["db_tables"])}**',f'- Tests: **{x["test_count"]}**',f'- Incident guardrails: **{len(x["incident_guardrails"])}**','']
open('docs/SUBSYSTEM_EXPLANATION_BRAIN.md','w').write('\n'.join(md)+'\n');print('subsystem brain',len(items),'commit',head[:12])
