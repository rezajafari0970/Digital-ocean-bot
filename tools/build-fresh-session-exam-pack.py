#!/usr/bin/env python3
import json,os,subprocess,hashlib
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
x=json.load(open('docs/BLIND_KNOWLEDGE_CHALLENGE.json',encoding='utf-8'))
# Deterministic stratified selection; public pack contains NO answer keys.
by={}
for q in x['challenges']:by.setdefault(q['category'],[]).append(q)
selected=[]
for cat,qs in sorted(by.items()):
 # select up to 5 spread items/category
 idx=[]
 for i in range(min(5,len(qs))):idx.append((i*max(1,len(qs)//5))%len(qs))
 for i in idx:selected.append(qs[i])
public=[{'exam_id':i+1,'category':q['category'],'question':q['question'],'evidence_sources':q['evidence']} for i,q in enumerate(selected)]
keys={str(i+1):q['answer_key'] for i,q in enumerate(selected)}
pack={'schema_version':1,'source_commit':head,'question_count':len(public),'rules':['Answer without opening the private key file.','Use project knowledge retrieval/source evidence; do not guess.','Record one JSON answer per exam_id.','Only after answers are frozen run the grader.'],'questions':public}
json.dump(pack,open('docs/FRESH_SESSION_EXAM_PACK.json','w'),ensure_ascii=False,indent=2);open('docs/FRESH_SESSION_EXAM_PACK.json','a').write('\n')
# Key hash in public metadata; actual key is separate local repo artifact for grading.
os.makedirs('.audit/fresh-session-exam',exist_ok=True)
key_path='.audit/fresh-session-exam/key.json'
json.dump({'schema_version':1,'source_commit':head,'keys':keys},open(key_path,'w'),ensure_ascii=False,indent=2);open(key_path,'a').write('\n')
h=hashlib.sha256(open(key_path,'rb').read()).hexdigest()
open('docs/FRESH_SESSION_EXAM_PACK.md','w').write(f'# Fresh Session Exam Pack\n\nCommit `{head}` — **{len(public)} stratified cross-layer questions**. The public pack contains no answer keys. Private grading key SHA256: `{h}`.\n\nThis pack is intended for a genuinely fresh session: answer first using Project Brain/source evidence, freeze answers, then grade.\n')
print('fresh exam pack',len(public),'categories',len(by),'key_sha256',h)
