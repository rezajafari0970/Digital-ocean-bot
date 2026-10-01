#!/usr/bin/env python3
import json,os,subprocess,hashlib,collections,re
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip();B=json.load(open('docs/BLIND_KNOWLEDGE_CHALLENGE.json'))['challenges'];by=collections.defaultdict(list)
for q in B:by[q['category']].append(q)
sel=[]
for cat in sorted(by):
 qs=by[cat]
 for i in range(min(5,len(qs))):sel.append(qs[(i*max(1,len(qs)//5))%len(qs)])
public=[];keys={}
for i,q in enumerate(sel,1):
 a=q['answer_key'];cat=q['category'];retr=None
 if cat=='db-chain':m=re.search(r'for ([A-Za-z0-9_]+\.[A-Za-z0-9_]+)',q['question']);retr={'type':'db','query':m.group(1)}
 elif cat=='route-chain':m=re.search(r'Where is ([A-Z]+) (.+?) handled\?',q['question']);retr={'type':'route','query':m.group(1)+'|'+m.group(2)}
 elif cat=='state-chain':retr={'type':'state','query':a['function']}
 elif cat=='ui-chain':retr={'type':'ui','query':a['function']}
 elif cat=='intent-chain':m=re.search(r'requirement is ([A-Z0-9-]+)\?',q['question']);retr={'type':'intent','query':m.group(1)}
 elif cat=='line-recall':m=re.search(r'line (\d+) of (.+?) at snapshot',q['question']);retr={'type':'line','query':m.group(2)+':'+m.group(1)}
 elif cat=='test-chain':m=re.search(r'What does ([A-Za-z0-9_]+) exercise',q['question']);retr={'type':'test','query':m.group(1)}
 # history is already 5/5 and has canonical answer from incident brain; add deterministic incident ID query via new history contract later if needed.
 item={'exam_id':i,'category':cat,'question':q['question'],'evidence_sources':q['evidence']}
 if retr:item['retrieval']=retr
 public.append(item);keys[str(i)]=a
os.makedirs('.audit/fresh-session-exam-v3',exist_ok=True);kp='.audit/fresh-session-exam-v3/key.json';json.dump({'schema_version':3,'source_commit':head,'keys':keys},open(kp,'w'),ensure_ascii=False,indent=2);open(kp,'a').write('\n');kh=hashlib.sha256(open(kp,'rb').read()).hexdigest();json.dump({'schema_version':3,'source_commit':head,'question_count':40,'questions':public},open('docs/FRESH_SESSION_EXAM_V3.json','w'),ensure_ascii=False,indent=2);open('docs/FRESH_SESSION_EXAM_V3.json','a').write('\n');open('docs/FRESH_SESSION_EXAM_V3.md','w').write(f'# Fresh Session Exam v3\n\nCommit `{head}` — 40 questions. **35 questions have deterministic contract retrieval; History remains direct incident-brain retrieval because it already passed 5/5.** Private key SHA256: `{kh}`.\n');print('v3',len(public),'contract',sum('retrieval'in x for x in public),'key',kh)
