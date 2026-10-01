#!/usr/bin/env python3
import json,os,subprocess,sys
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
artifacts=['docs/FULL_SOURCE_KNOWLEDGE.json','docs/SEMANTIC_KNOWLEDGE.json','docs/DEEP_REQUIREMENTS_BRAIN.json','docs/FUNCTION_BEHAVIOR_BRAIN.json','docs/COLUMN_BEHAVIOR_BRAIN.json','docs/TEST_BEHAVIOR_BRAIN.json','docs/STATE_TRANSITION_BRAIN.json','docs/FRONTEND_ACTION_STATE_BRAIN.json','docs/INCIDENT_REGRESSION_BRAIN.json','docs/BLIND_KNOWLEDGE_CHALLENGE.json']
rows=[]
for p in artifacts:
 if not os.path.exists(p):rows.append({'artifact':p,'status':'missing'});continue
 d=json.load(open(p,encoding='utf-8')); c=d.get('source_commit')
 if not c:rows.append({'artifact':p,'status':'unversioned'});continue
 anc=subprocess.run(['git','merge-base','--is-ancestor',c,head]).returncode==0
 dist=None
 if anc:
  dist=int(subprocess.check_output(['git','rev-list','--count',f'{c}..{head}'],text=True).strip())
 # Knowledge-only commits can advance HEAD without changing indexed source. Detect relevant source drift.
 changed=[]
 if anc and c!=head:
  out=subprocess.check_output(['git','diff','--name-only',c,head,'--','cmd','internal','migrations','web/static','deploy'],text=True).splitlines();changed=out
 status='current-source' if anc and not changed else ('stale-source' if anc else 'diverged')
 rows.append({'artifact':p,'source_commit':c,'head_distance':dist,'relevant_changed_files':changed[:100],'status':status})
ready=all(x['status']=='current-source' for x in rows)
out={'schema_version':1,'git_head':head,'ready':ready,'artifacts':rows,'rule':'An artifact may be commits behind HEAD only when no indexed product/source paths changed since its source_commit. Knowledge/docs-only commits do not make source knowledge stale.'}
json.dump(out,open('docs/KNOWLEDGE_DRIFT_STATUS.json','w'),indent=2);open('docs/KNOWLEDGE_DRIFT_STATUS.json','a').write('\n')
print('KNOWLEDGE DRIFT', 'PASS' if ready else 'FAIL','head',head[:12])
for x in rows:print(x['status'],x['artifact'],'distance='+str(x.get('head_distance')),'changed='+str(len(x.get('relevant_changed_files',[]))))
sys.exit(0 if ready else 1)
