#!/usr/bin/env python3
import subprocess,json,sys,os
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT)
steps=[('drift',['python3','tools/knowledge-drift-gate.py']),('handoff',['python3','tools/handoff-acceptance-100.py']),('deep_mastery',['python3','tools/deep-mastery-exam.py']),('blind_structure',['python3','tools/validate-blind-knowledge-challenge.py'])]
results=[]
for name,cmd in steps:
 p=subprocess.run(cmd,text=True,capture_output=True);results.append({'name':name,'pass':p.returncode==0,'stdout':p.stdout[-3000:],'stderr':p.stderr[-1000:]})
v3=json.load(open('docs/MASTER_KNOWLEDGE_SCORE_V3.json'))
results.append({'name':'master_v3','pass':v3.get('knowledge_ready') is True and v3.get('knowledge_mastery_percent')==100.0,'stdout':f"score={v3.get('knowledge_mastery_percent')} ready={v3.get('knowledge_ready')}",'stderr':''})
ready=all(x['pass'] for x in results);out={'ready':ready,'checks':results}
json.dump(out,open('docs/FRESH_CHAT_MASTERY_STATUS.json','w'),indent=2);open('docs/FRESH_CHAT_MASTERY_STATUS.json','a').write('\n')
print('FRESH CHAT MASTERY', 'READY' if ready else 'NOT READY')
for x in results:print(('PASS' if x['pass'] else 'FAIL'),x['name'],x['stdout'].splitlines()[-1] if x['stdout'].splitlines() else '')
sys.exit(0 if ready else 1)
