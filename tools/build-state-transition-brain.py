#!/usr/bin/env python3
import json,os,re,subprocess,collections
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
S=json.load(open('docs/FULL_SOURCE_KNOWLEDGE.json',encoding='utf-8')); F=json.load(open('docs/FUNCTION_BEHAVIOR_BRAIN.json',encoding='utf-8')); L=json.load(open('docs/LIFECYCLE_STATE_BRAIN.json',encoding='utf-8'))
source={x['path']:x for x in S['files']}; states=set(L.get('states',{})); trans=[]
for fn in F['functions']:
 f=source.get(fn['file']);
 if not f:continue
 lines=f['lines'][fn['start_line']-1:fn['end_line']]; body='\n'.join(x['text'] for x in lines)
 mentioned=sorted(s for s in states if re.search(r'["\']'+re.escape(s)+r'["\']',body))
 if not mentioned:continue
 writes=[]
 for x in lines:
  t=x['text']
  if re.search(r'(?i)(UPDATE\s+\w+\s+SET.*state|state\s*=|State\s*:)',t):
   vals=[s for s in mentioned if s in t]; writes.append({'line':x['n'],'text':t.strip()[:500],'states':vals})
 guards=[]
 for x in lines:
  t=x['text']
  if ('state' in t.lower()) and re.search(r'\b(if|switch|case|WHERE)\b',t,re.I): guards.append({'line':x['n'],'text':t.strip()[:500]})
 timing=[]
 for x in lines:
  if re.search(r'time\.(?:Sleep|After|NewTicker|NewTimer)|Retry|Backoff',x['text'],re.I):timing.append({'line':x['n'],'text':x['text'].strip()[:500]})
 if writes or guards:
  trans.append({'owner_function':fn['name'],'file':fn['file'],'start_line':fn['start_line'],'end_line':fn['end_line'],'states_mentioned':mentioned,'transition_write_evidence':writes,'guard_evidence':guards[:20],'retry_timing_evidence':timing[:20],'db_reads':fn.get('db_reads',[]),'db_writes':fn.get('db_writes',[]),'candidate_callers':fn.get('candidate_caller_files',[])})
# state classification from observed names only
terminal=[]
for s in sorted(states):
 if any(k in s for k in ['FAILED','COMPLETE','DELETED','READY']):terminal.append(s)
out={'schema_version':1,'source_commit':head,'methodology':'static evidence for state guards/writes and owners; from→to direction is not asserted unless explicit in source','state_count':len(states),'owner_count':len(trans),'terminal_like_states':terminal,'owners':trans}
json.dump(out,open('docs/STATE_TRANSITION_BRAIN.json','w'),ensure_ascii=False,indent=2);open('docs/STATE_TRANSITION_BRAIN.json','a').write('\n')
we=sum(bool(x['transition_write_evidence']) for x in trans);gu=sum(bool(x['guard_evidence']) for x in trans)
md=['# State Transition Brain','',f'Commit `{head}` — **{len(states)} known state-like values**, **{len(trans)} owner functions with transition/guard evidence**.','',f'- Owners with explicit state-write evidence: **{we}**',f'- Owners with guard/precondition evidence: **{gu}**',f'- Terminal-like states by naming evidence: **{len(terminal)}**','', 'This brain identifies owner functions, exact source ranges, state mentions, explicit write evidence, guards/preconditions, retry/timing evidence and DB touch points. It intentionally does not fabricate a from→to edge when source does not make direction explicit.']
open('docs/STATE_TRANSITION_BRAIN.md','w').write('\n'.join(md)+'\n');print('state transition',len(states),'owners',len(trans),'writes',we,'guards',gu,'commit',head[:12])
