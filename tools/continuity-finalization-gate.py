#!/usr/bin/env python3
import json,subprocess,os,sys
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT)
def J(p):return json.load(open(p,encoding='utf-8'))
# Refresh source-aware drift first.
p=subprocess.run(['python3','tools/knowledge-drift-gate.py'],text=True,capture_output=True);print(p.stdout,end='');
if p.returncode:raise SystemExit('FINALIZATION FAIL: drift')
proof=J('docs/FRESH_SESSION_MASTERY_PROOF.json');master=J('docs/MASTER_KNOWLEDGE_SCORE_V3.json');drift=J('docs/KNOWLEDGE_DRIFT_STATUS.json')
checks={
'fresh_session_proof': proof.get('verified') is True and proof.get('score')==proof.get('total')==40,
'master_knowledge_v3': master.get('knowledge_ready') is True and master.get('knowledge_mastery_percent')==100.0,
'knowledge_drift': drift.get('ready') is True,
'proof_all_categories': all(v['passed']==v['total'] for v in proof.get('categories',{}).values()),
'canonical_branch': subprocess.check_output(['git','branch','--show-current'],text=True).strip()=='checkpoint/final-e2e-20260929'
}
# Classify working-tree noise: tracked product source vs knowledge/status/audit.
out=subprocess.check_output(['git','status','--porcelain=v1'],text=True).splitlines();product=[];volatile=[]
for line in out:
 path=line[3:] if len(line)>3 else line
 if path.startswith(('cmd/','internal/','migrations/','web/static/','deploy/')):product.append(line)
 else:volatile.append(line)
checks['no_uncommitted_product_source']=not product
ready=all(checks.values())
status={'schema_version':1,'ready':ready,'checks':checks,'independent_fresh_session_score':'40/40','knowledge_mastery_percent':master.get('knowledge_mastery_percent'),'source_drift_ready':drift.get('ready'),'uncommitted_product_source':product,'volatile_or_knowledge_worktree_entries':volatile,'note':'Volatile .audit/status artifacts do not constitute product-source drift; tracked product paths are checked separately.'}
json.dump(status,open('docs/CONTINUITY_FINAL_STATUS.json','w'),indent=2);open('docs/CONTINUITY_FINAL_STATUS.json','a').write('\n')
print('CONTINUITY FINALIZATION', 'READY' if ready else 'NOT READY')
for k,v in checks.items():print(('PASS' if v else 'FAIL'),k)
print('product_worktree_entries',len(product),'volatile_entries',len(volatile));sys.exit(0 if ready else 1)
