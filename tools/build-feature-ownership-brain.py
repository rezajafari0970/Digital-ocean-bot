#!/usr/bin/env python3
import json,os,subprocess,re,collections
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
def J(p):return json.load(open(p,encoding='utf-8'))
D=J('docs/DEEP_REQUIREMENTS_BRAIN.json'); FB=J('docs/FUNCTION_BEHAVIOR_BRAIN.json'); CB=J('docs/COLUMN_BEHAVIOR_BRAIN.json'); TB=J('docs/TEST_BEHAVIOR_BRAIN.json'); UI=J('docs/FRONTEND_ACTION_STATE_BRAIN.json'); SY=J('docs/SYMBOL_INDEX.json'); IR=J('docs/INCIDENT_REGRESSION_BRAIN.json')
features={
'accounts':['account','accounts'],'digitalocean':['digitalocean','droplet'],'vultr':['vultr','console','capacity'],'proxy_network':['proxy','network','sticky','geo'],'lifecycle':['lifecycle','deploy','droplet','scheduler','reconcile'],'provisioning':['provision','installer','ssh'],'sanaei_panel':['sanaei','panel','inbound','client'],'reality':['reality','sni','target'],'config_users':['config','user','quota','lifetime','device','capacity'],'output':['output','share','snapshot','visible_until'],'residential':['residential'],'database':['migration','schema','database'],'admin_api':['adminapi','/api/v1/'],'frontend':['frontend','app.js','data-action'],'tests':['test'],'deployment_runtime':['runtime','deploy','worker','health','version'],'architecture':['provider','registry','factory'],'continuity':['brain','handoff','knowledge']}
items=[]
for feat,terms in features.items():
 def hit(s):
  z=s.lower();return any(t.lower() in z for t in terms)
 req=[x for x in D['requirements'] if x['area']==feat or hit(x['requirement'])]
 routes=[]
 for r in SY['routes']:
  blob=r['path']+' '+r['handler']+' '+((r.get('handler_definition') or {}).get('file',''))
  if hit(blob):routes.append({'method':r['method'],'path':r['path'],'handler':r['handler']})
 funcs=[]
 for x in FB['functions']:
  blob=x['name']+' '+x['file']+' '+x['package']+' '+' '.join(x.get('db_reads',[])+x.get('db_writes',[]))
  if hit(blob):funcs.append({'name':x['name'],'file':x['file'],'line':x['start_line'],'package':x['package']})
 cols=[]
 for x in CB['columns']:
  if hit(x['table']+'.'+x['column']):cols.append({'table':x['table'],'column':x['column'],'added_by':x.get('added_by')})
 tests=[]
 for x in TB['tests']:
  if hit(x['test']+' '+x['file']+' '+x['package']):tests.append({'name':x['test'],'file':x['file'],'line':x['start_line']})
 uif=[]
 for x in UI['functions']:
  blob=x['name']+' '+' '.join(x['api_literals']+x['dom_selectors'])
  if hit(blob):uif.append({'name':x['name'],'line':x['start_line'],'apis':x['api_literals'],'dom':x['dom_selectors']})
 incidents=[x['id'] for x in IR['incidents'] if x['area']==feat or hit(x['area']+' '+x['failure']+' '+x['lesson'])]
 items.append({'feature':feat,'matching_terms':terms,'requirements':[{'id':x['id'],'text':x['requirement']} for x in req],'api_routes':routes,'functions':funcs[:300],'db_columns':cols[:500],'tests':tests[:200],'frontend_functions':uif[:100],'incident_guardrails':incidents,'counts':{'requirements':len(req),'routes':len(routes),'functions':len(funcs),'columns':len(cols),'tests':len(tests),'frontend_functions':len(uif),'incidents':len(incidents)}})
out={'schema_version':1,'source_commit':head,'feature_count':len(items),'features':items,'methodology':'cross-package feature ownership via explicit feature vocabulary over requirements/routes/functions/columns/tests/frontend/incidents; matches are navigation evidence and exact source remains authoritative'}
json.dump(out,open('docs/FEATURE_OWNERSHIP_BRAIN.json','w'),ensure_ascii=False,indent=2);open('docs/FEATURE_OWNERSHIP_BRAIN.json','a').write('\n')
md=['# Feature / Cross-Ownership Brain','',f'Commit `{head}` — **{len(items)} feature-level ownership views** across physical package boundaries.','', 'This fixes the package-vs-feature gap: a feature can own UI actions, admin API routes, provider/network functions, DB columns and tests even when those live in different packages. Matching is vocabulary-based navigation evidence, not an assertion of exclusive ownership.','', '| Feature | Req | Routes | Funcs | Columns | Tests | UI | Incidents |','|---|---:|---:|---:|---:|---:|---:|---:|']
for x in items:
 c=x['counts'];md.append(f"| `{x['feature']}` | {c['requirements']} | {c['routes']} | {c['functions']} | {c['columns']} | {c['tests']} | {c['frontend_functions']} | {c['incidents']} |")
open('docs/FEATURE_OWNERSHIP_BRAIN.md','w').write('\n'.join(md)+'\n');print('feature ownership',len(items),'commit',head[:12]);[print(x['feature'],x['counts']) for x in items]
