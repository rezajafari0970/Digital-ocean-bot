#!/usr/bin/env python3
import json,os,subprocess,hashlib,collections
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(); B=json.load(open('docs/BLIND_KNOWLEDGE_CHALLENGE.json'))['challenges']
by=collections.defaultdict(list)
for q in B:by[q['category']].append(q)
sel=[]
for cat in sorted(by):
 qs=by[cat]
 for i in range(min(5,len(qs))):sel.append(qs[(i*max(1,len(qs)//5))%len(qs)])
public=[];keys={}
for i,q in enumerate(sel,1):
 item={'exam_id':i,'category':q['category'],'question':q['question'],'evidence_sources':q['evidence']}
 a=q['answer_key']; cat=q['category']
 # Public deterministic retrieval hints only where contract applies; no answers exposed.
 if cat=='db-chain':
  import re;m=re.search(r'for ([A-Za-z0-9_]+\.[A-Za-z0-9_]+)',q['question']); item['retrieval']={'type':'db','query':m.group(1) if m else ''}
 elif cat=='route-chain':
  import re;m=re.search(r'Where is ([A-Z]+) (.+?) handled\?',q['question']); item['retrieval']={'type':'route','query':(m.group(1)+'|'+m.group(2)) if m else ''}
 elif cat=='state-chain':item['retrieval']={'type':'state','query':a['function']}
 elif cat=='ui-chain':item['retrieval']={'type':'ui','query':a['function']}
 public.append(item);keys[str(i)]=a
pack={'schema_version':2,'source_commit':head,'question_count':len(public),'contract':'docs/KNOWLEDGE_RETRIEVAL_CONTRACT.md','questions':public}
os.makedirs('.audit/fresh-session-exam-v2',exist_ok=True);kp='.audit/fresh-session-exam-v2/key.json';json.dump({'schema_version':2,'source_commit':head,'keys':keys},open(kp,'w'),ensure_ascii=False,indent=2);open(kp,'a').write('\n');kh=hashlib.sha256(open(kp,'rb').read()).hexdigest();json.dump(pack,open('docs/FRESH_SESSION_EXAM_V2.json','w'),ensure_ascii=False,indent=2);open('docs/FRESH_SESSION_EXAM_V2.json','a').write('\n');open('docs/FRESH_SESSION_EXAM_V2.md','w').write(f'# Fresh Session Exam v2\n\nCommit `{head}` — **{len(public)} questions** with deterministic retrieval-contract hints for ambiguous cross-layer categories. Private key SHA256: `{kh}`.\n')
print('exam v2',len(public),'key',kh)
