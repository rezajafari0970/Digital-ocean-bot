#!/usr/bin/env python3
import json,os,subprocess
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
def J(p):return json.load(open(p,encoding='utf-8'))
FO=J('docs/FEATURE_OWNERSHIP_BRAIN.json'); SE=J('docs/SUBSYSTEM_EXPLANATION_BRAIN.json'); D=J('docs/DEEP_REQUIREMENTS_BRAIN.json'); FB=J('docs/FUNCTION_BEHAVIOR_BRAIN.json'); CB=J('docs/COLUMN_BEHAVIOR_BRAIN.json'); TB=J('docs/TEST_BEHAVIOR_BRAIN.json'); IR=J('docs/INCIDENT_REGRESSION_BRAIN.json')
gaps=[]
def add(feature,dimension,severity,reason,evidence):gaps.append({'feature':feature,'dimension':dimension,'severity':severity,'reason':reason,'evidence':evidence})
for x in FO['features']:
 c=x['counts']; f=x['feature']
 # Only require dimensions that make conceptual sense; zeros are signals, not automatically defects.
 if c['requirements']==0:add(f,'intent','high','no direct requirement evidence',c)
 if f not in ['continuity','frontend','database'] and c['functions']==0:add(f,'implementation','high','no function evidence',c)
 if f in ['accounts','vultr','proxy_network','provisioning','sanaei_panel','config_users','output','residential','admin_api'] and c['routes']==0:add(f,'api','medium','no directly matched API route',c)
 if f not in ['continuity','frontend','tests','admin_api'] and c['columns']==0:add(f,'database','medium','no directly matched DB column',c)
 if f not in ['continuity','frontend'] and c['tests']==0:add(f,'tests','high','no directly matched test evidence',c)
 if f in ['accounts','vultr','proxy_network','provisioning','sanaei_panel','config_users','output','residential'] and c['frontend_functions']==0:add(f,'frontend','low','no directly matched frontend function',c)
# Static evidence quality gaps
stats=CB['statistics']
if stats['columns_with_migration_origin']<CB['column_count']:add('database','migration-origin','medium',f"{CB['column_count']-stats['columns_with_migration_origin']} live columns lack parsed migration-origin evidence",stats)
if stats['columns_with_function_mentions']<CB['column_count']:add('database','code-mentions','medium',f"{CB['column_count']-stats['columns_with_function_mentions']} live columns lack direct function mention evidence",stats)
# Tests with no subtests are not gaps; tests without assertions/calls are.
na=sum(not x['assertion_evidence'] for x in TB['tests']); nc=sum(not x['candidate_calls'] for x in TB['tests'])
if na:add('tests','assertions','high',f'{na} tests lack assertion evidence',{})
if nc:add('tests','calls','medium',f'{nc} tests lack candidate call evidence',{})
# Incidents: features with known high-risk domains but no incident guardrail evidence
for f in ['lifecycle','sanaei_panel','reality','residential','admin_api','frontend']:
 x=next(z for z in FO['features'] if z['feature']==f)
 if x['counts']['incidents']==0:add(f,'history','low','no dedicated incident guardrail currently encoded',x['counts'])
summary={'total_gaps':len(gaps),'high':sum(x['severity']=='high' for x in gaps),'medium':sum(x['severity']=='medium' for x in gaps),'low':sum(x['severity']=='low' for x in gaps)}
out={'schema_version':1,'source_commit':head,'summary':summary,'gaps':gaps,'rule':'A gap means direct knowledge evidence is weak/missing for that dimension; it is not automatically a product bug or missing implementation.'}
json.dump(out,open('docs/KNOWLEDGE_COVERAGE_GAPS.json','w'),ensure_ascii=False,indent=2);open('docs/KNOWLEDGE_COVERAGE_GAPS.json','a').write('\n')
md=['# Knowledge Coverage Gap Brain','',f'Commit `{head}` — **{summary["total_gaps"]} knowledge-evidence gaps**: {summary["high"]} high, {summary["medium"]} medium, {summary["low"]} low.','', 'These are gaps in direct evidence available to a fresh chat, not automatically defects in the product. They identify where understanding still relies on indirect inference.','', '| Feature | Dimension | Severity | Reason |','|---|---|---|---|']
for x in gaps:md.append(f"| `{x['feature']}` | `{x['dimension']}` | **{x['severity']}** | {x['reason']} |")
open('docs/KNOWLEDGE_COVERAGE_GAPS.md','w').write('\n'.join(md)+'\n');print('coverage gaps',summary);[print(x['severity'],x['feature'],x['dimension'],x['reason']) for x in gaps]
