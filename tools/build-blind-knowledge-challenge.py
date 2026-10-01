#!/usr/bin/env python3
import json,os,subprocess,collections,random
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
def J(p):return json.load(open(p,encoding='utf-8'))
D=J('docs/DEEP_REQUIREMENTS_BRAIN.json'); FB=J('docs/FUNCTION_BEHAVIOR_BRAIN.json'); CB=J('docs/COLUMN_BEHAVIOR_BRAIN.json'); TB=J('docs/TEST_BEHAVIOR_BRAIN.json'); ST=J('docs/STATE_TRANSITION_BRAIN.json'); UI=J('docs/FRONTEND_ACTION_STATE_BRAIN.json'); IR=J('docs/INCIDENT_REGRESSION_BRAIN.json'); SY=J('docs/SYMBOL_INDEX.json'); SRC=J('docs/FULL_SOURCE_KNOWLEDGE.json')
ch=[]
def add(cat,q,answer,evidence):ch.append({'id':len(ch)+1,'category':cat,'question':q,'answer_key':answer,'evidence':evidence})
# Cross-layer deterministic questions, answers generated from evidence rather than prose guesses.
for r in SY['routes'][:20]:
 h=r.get('handler_definition')
 if h:add('route-chain',f"Where is {r['method']} {r['path']} handled?",{'handler':r['handler'],'file':h['file'],'line':h['line']},['SYMBOL_INDEX'])
for x in [z for z in CB['columns'] if z['reader_functions'] or z['writer_functions']][:20]:
 add('db-chain',f"What is the origin and code evidence for {x['table']}.{x['column']}?",{'added_by':x['added_by'],'readers':x['reader_functions'][:3],'writers':x['writer_functions'][:3]},['COLUMN_BEHAVIOR_BRAIN','SCHEMA_INDEX'])
for x in ST['owners'][:15]:
 add('state-chain',f"Which function owns state evidence involving {', '.join(x['states_mentioned'][:3])}?",{'function':x['owner_function'],'file':x['file'],'line':x['start_line'],'writes':x['transition_write_evidence'][:2],'guards':x['guard_evidence'][:2]},['STATE_TRANSITION_BRAIN','FULL_SOURCE_KNOWLEDGE'])
for x in UI['functions']:
 if x['api_literals']:
  add('ui-chain',f"Which frontend function calls {x['api_literals'][0]} and what UI evidence surrounds it?",{'function':x['name'],'file':x['file'],'line':x['start_line'],'dom':x['dom_selectors'][:4],'loading':x['loading_evidence'][:3],'errors':x['error_evidence'][:3]},['FRONTEND_ACTION_STATE_BRAIN']);
 if len([z for z in ch if z['category']=='ui-chain'])>=15:break
for x in IR['incidents']:
 add('history-chain',f"What regression guardrail applies to {x['failure']}?",{'lesson':x['lesson'],'prevent':x['prevent']},['INCIDENT_REGRESSION_BRAIN','HISTORICAL_DECISION_LEDGER'])
for x in TB['tests'][:10]:
 add('test-chain',f"What does {x['test']} exercise structurally?",{'file':x['file'],'line':x['start_line'],'calls':x['candidate_calls'][:6],'assertions':x['assertion_evidence'][:3]},['TEST_BEHAVIOR_BRAIN','FULL_SOURCE_KNOWLEDGE'])
for x in D['requirements'][:10]:add('intent-chain',f"What durable requirement is {x['id']}?",{'area':x['area'],'requirement':x['requirement'],'parents':x['parent_requirement_ids']},['DEEP_REQUIREMENTS_BRAIN'])
# exact line challenge samples, deterministic spread
files=[f for f in SRC['files'] if f['path'].endswith(('.go','.js','.sql')) and f['line_count']>=20]
for idx in [3,17,31,47,63,79,101,127,151,181]:
 f=files[idx%len(files)]; line=max(1,min(f['line_count'],(idx*37)%f['line_count']+1)); text=f['lines'][line-1]['text'];add('line-recall',f"What is exact line {line} of {f['path']} at snapshot commit?",{'text':text,'sha256':f['sha256'],'commit':SRC['source_commit']},['FULL_SOURCE_KNOWLEDGE'])
out={'schema_version':1,'source_commit':head,'challenge_count':len(ch),'categories':dict(collections.Counter(x['category'] for x in ch)),'challenges':ch,'rule':'Answers are evidence keys for a fresh-session blind challenge. Passing requires exact retrieval/cross-layer composition; no answer should be guessed from memory.'}
json.dump(out,open('docs/BLIND_KNOWLEDGE_CHALLENGE.json','w'),ensure_ascii=False,indent=2);open('docs/BLIND_KNOWLEDGE_CHALLENGE.json','a').write('\n')
md=['# Blind Cross-Layer Knowledge Challenge','',f'Commit `{head}` — **{len(ch)} evidence-keyed challenges**.','', 'These questions require cross-layer retrieval/composition rather than merely checking that an artifact exists. Answer keys are generated from commit-pinned knowledge evidence.','', '## Categories']+[f'- `{k}` — {v}' for k,v in sorted(out['categories'].items())]
open('docs/BLIND_KNOWLEDGE_CHALLENGE.md','w').write('\n'.join(md)+'\n');print('blind challenge',len(ch),out['categories'],'commit',head[:12])
