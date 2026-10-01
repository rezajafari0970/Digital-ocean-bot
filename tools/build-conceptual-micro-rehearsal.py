#!/usr/bin/env python3
import json,os,math
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);M=json.load(open('docs/ACTIVE_RECALL_REHEARSAL_MANIFEST.json'))['items'];os.makedirs('docs/source-memory-rehearsal-micro',exist_ok=True);out=[]
# conceptual units 9..21; split each original pack/recall/correction by aligned symbol/question chunks of 20.
for unit in range(9,22):
 z=M[unit-1]; pack=json.load(open(z['pack'])); recall=json.load(open(z['recall']))['questions']; corr=json.load(open(z['correction']))['answers']; assert len(pack)==len(recall)==len(corr)
 for mi,start in enumerate(range(0,len(pack),20),1):
  pc=pack[start:start+20]; rc=recall[start:start+20]; # renumber local ids, align corrections
  localq=[];locala={}
  for j,q in enumerate(rc,1):
   old=str(q['id']);localq.append({'id':j,'prompt':q['prompt']});locala[str(j)]=corr[old]
  base=f'unit-{unit:02d}-micro-{mi:02d}'
  pp=f'docs/source-memory-rehearsal-micro/{base}-pack.json';rp=f'docs/source-memory-rehearsal-micro/{base}-recall.json';cp=f'docs/source-memory-rehearsal-micro/{base}-correction.json'
  json.dump(pc,open(pp,'w'),ensure_ascii=False,separators=(',',':'));open(pp,'a').write('\n');json.dump({'questions':localq},open(rp,'w'),ensure_ascii=False,separators=(',',':'));open(rp,'a').write('\n');json.dump({'answers':locala},open(cp,'w'),ensure_ascii=False,separators=(',',':'));open(cp,'a').write('\n')
  out.append({'unit':unit,'micro':mi,'pack':pp,'recall':rp,'correction':cp,'questions':len(localq)})
json.dump({'schema_version':1,'micro_units':out},open('docs/CONCEPTUAL_MICRO_REHEARSAL_MANIFEST.json','w'),indent=2);open('docs/CONCEPTUAL_MICRO_REHEARSAL_MANIFEST.json','a').write('\n');print('micro units',len(out),'max_questions',max(x['questions'] for x in out));
for u in range(9,22):print('unit',u,'micro',sum(x['unit']==u for x in out))
