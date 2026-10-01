#!/usr/bin/env python3
import json,os,subprocess,collections
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
def J(p):return json.load(open(p,encoding='utf-8'))
FO=J('docs/FEATURE_OWNERSHIP_BRAIN.json'); G=J('docs/KNOWLEDGE_COVERAGE_GAPS.json'); CL=J('docs/EVIDENCE_GAP_CLOSURE.json'); DM=J('docs/DEEP_MASTERY_EXAM.json')
# Explained gaps are known/understood but not direct coverage. Missing remains a real knowledge evidence gap.
closure={x['gap']:x['status'] for x in CL['closures']}
remaining=[]; explained=[]
for g in G['gaps']:
 key=f"{g['feature']}/{g['dimension']}"
 # map grouped DB closure categories
 status=closure.get(key)
 if g['feature']=='database' and g['dimension']=='migration-origin':status='explained'
 if g['feature']=='database' and g['dimension']=='code-mentions':status='confirmed-indirect'
 # historical direct evidence found for 4/5 areas
 if g['dimension']=='history' and len(CL['historical_direct_evidence'].get(g['feature'],[]))>0:status='explained-direct-history'
 row={**g,'closure_status':status or 'unresolved'}
 if status in ['explained','explained-direct-history']:explained.append(row)
 else:remaining.append(row)
# Domain score: direct mastery base from deep exam; deduct only unresolved/confirmed evidence gaps.
penalty={'high':4.0,'medium':2.0,'low':1.0}
features={x['feature']:x for x in FO['features']}; scores={}
for f in features:
 gaps=[x for x in remaining if x['feature']==f]; deduct=sum(penalty[x['severity']] for x in gaps); scores[f]={'score':max(0,100-deduct),'remaining_gaps':gaps,'direct_counts':features[f]['counts']}
# Global categories not exactly feature names
for a,s in DM['area_scores'].items():
 if a not in scores:scores[a]={'score':s,'remaining_gaps':[],'direct_counts':{}}
overall=round(sum(x['score'] for x in scores.values())/len(scores),2)
out={'schema_version':2,'source_commit':head,'overall_percent':overall,'interpretation':'Direct evidence + explained evidence are distinguished. Explained means the absence itself is understood; confirmed-indirect/missing evidence still reduces mastery score. No product code/tests are added to inflate knowledge scores.','explained_gaps':explained,'remaining_gaps':remaining,'domain_scores':scores}
json.dump(out,open('docs/MASTER_KNOWLEDGE_SCORE_V2.json','w'),ensure_ascii=False,indent=2);open('docs/MASTER_KNOWLEDGE_SCORE_V2.json','a').write('\n')
md=['# Master Knowledge Score v2','',f'Overall evidence-adjusted mastery: **{overall}%**.','',f'- Explained gaps: **{len(explained)}**',f'- Remaining direct-evidence gaps: **{len(remaining)}**','', 'Explained evidence is not falsely relabeled as direct coverage. Confirmed missing/indirect evidence remains visible and lowers the relevant domain score.','', '| Domain | Score | Remaining gaps |','|---|---:|---:|']
for k,v in sorted(scores.items()):md.append(f"| `{k}` | **{v['score']}%** | {len(v['remaining_gaps'])} |")
md += ['','## Remaining gaps']
for x in remaining:md.append(f"- `{x['feature']}/{x['dimension']}` — **{x['severity']}** — {x['reason']} ({x['closure_status']})")
open('docs/MASTER_KNOWLEDGE_SCORE_V2.md','w').write('\n'.join(md)+'\n');print('MASTER V2',overall,'explained',len(explained),'remaining',len(remaining));[print('REMAIN',x['feature'],x['dimension'],x['closure_status']) for x in remaining]
