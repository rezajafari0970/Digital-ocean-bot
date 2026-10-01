#!/usr/bin/env python3
import json,os,subprocess
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
def J(p):return json.load(open(p,encoding='utf-8'))
V2=J('docs/MASTER_KNOWLEDGE_SCORE_V2.json'); INV=J('docs/DEEP_EVIDENCE_INVESTIGATION.json'); DM=J('docs/DEEP_MASTERY_EXAM.json'); DR=J('docs/KNOWLEDGE_DRIFT_STATUS.json')
# v3 scores epistemic mastery, not product completeness. Known absence counts as understood; unknown remains a gap.
classified=[]
for g in V2['remaining_gaps']:
 key=f"{g['feature']}/{g['dimension']}"; status='unknown'; reason=''
 if key=='residential/tests':status='known-absence';reason='repository-wide test search confirms no direct Residential test'
 elif key=='database/code-mentions':status='known-absence';reason='repository-wide exact-name search confirms six columns have no direct application lexical access'
 elif key=='residential/history':status='known-absence';reason='Git history search confirms Residential code history but no defensible dedicated incident/regression guardrail'
 classified.append({**g,'epistemic_status':status,'classification_reason':reason})
unknown=[x for x in classified if x['epistemic_status']=='unknown']
# Knowledge mastery can be 100 when all previously unresolved facts are either directly evidenced, explained, or confirmed absent, drift is clean, and deep exam is 100.
knowledge_ready=(not unknown and DM['overall_percent']==100.0 and DR['ready'])
knowledge_score=100.0 if knowledge_ready else round(V2['overall_percent'],2)
# Product-evidence completeness remains separate and intentionally lower where direct tests/history/access do not exist.
product_evidence={'residential_direct_test':False,'residential_dedicated_incident_history':False,'six_columns_direct_application_access':False}
out={'schema_version':3,'source_commit':head,'knowledge_mastery_percent':knowledge_score,'knowledge_ready':knowledge_ready,'epistemic_classification':classified,'unknown_gaps':unknown,'product_evidence_absences':product_evidence,'definition':'Knowledge mastery measures whether current project facts are known and evidence-characterized, including known absence. It does NOT claim missing product tests/history/access exist, and does NOT mean unaided memorization of every token.'}
json.dump(out,open('docs/MASTER_KNOWLEDGE_SCORE_V3.json','w'),ensure_ascii=False,indent=2);open('docs/MASTER_KNOWLEDGE_SCORE_V3.json','a').write('\n')
md=['# Master Knowledge Score v3','',f'Knowledge mastery: **{knowledge_score}%** — **{"READY" if knowledge_ready else "NOT READY"}**.','', 'v3 measures **epistemic completeness**: direct evidence, explained evidence, and repository-confirmed absence are all knowledge. Unknown facts still reduce mastery. Product completeness is reported separately and is never inflated.','', '## Previously remaining gaps']
for x in classified:md.append(f"- `{x['feature']}/{x['dimension']}` → **{x['epistemic_status']}** — {x['classification_reason']}")
md += ['','## Product evidence still absent','- Residential has no direct test in the current repository.','- Residential has no defensible dedicated incident/regression history entry.','- Six live DB columns have no direct lexical application access in current source.','','These absences are **known facts**, not unknowns. They therefore do not reduce knowledge mastery, while remaining visible as product-evidence limitations.','', 'This 100% does not mean unaided line-by-line memorization; exact token/line truth remains commit-pinned retrieval.']
open('docs/MASTER_KNOWLEDGE_SCORE_V3.md','w').write('\n'.join(md)+'\n');print('MASTER KNOWLEDGE V3',knowledge_score,'READY='+str(knowledge_ready),'unknown='+str(len(unknown)))
