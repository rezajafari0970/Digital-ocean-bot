#!/usr/bin/env python3
import glob,hashlib,json,os,tempfile
from datetime import datetime,timezone
ROOT=os.path.dirname(os.path.dirname(os.path.abspath(__file__))); RD=os.path.join(ROOT,'.local/spaced-results'); OUT=os.path.join(ROOT,'.local/spaced-mastery-v2.json')
def canon_id(card):
 raw=json.dumps({'kind':card['kind'],'cue':card['cue']},sort_keys=True,separators=(',',':'),ensure_ascii=False)
 return hashlib.sha256(raw.encode()).hexdigest()[:24]
def atomic(path,obj):
 os.makedirs(os.path.dirname(path),exist_ok=True);fd,tmp=tempfile.mkstemp(dir=os.path.dirname(path),prefix='.mastery2-',text=True)
 with os.fdopen(fd,'w',encoding='utf-8') as f:json.dump(obj,f,ensure_ascii=False,indent=2);f.write('\n');f.flush();os.fsync(f.fileno())
 os.replace(tmp,path)
state={'schema_version':2,'identity':'sha256(kind+canonical cue)','cards':{},'processed_results':[],'updated_at':None}
rows=[]
for rp in glob.glob(os.path.join(RD,'pass-*.json')):
 try:r=json.load(open(rp,encoding='utf-8'))
 except:continue
 if r.get('status')!='COMPLETE':continue
 cur=r['cursor']; cp=os.path.join(ROOT,cur['path'])
 try:cards=json.load(open(cp,encoding='utf-8'))
 except:continue
 for item in r.get('cards',[]):
  n=item['card'];
  if not (1<=n<=len(cards)):continue
  card=cards[n-1];rows.append((str(cur['pass']),cur['unit'],cur['micro'],n,os.path.basename(rp),card,item))
# deterministic pass order: 1,2,3,V1,V2,V3
def pk(p):
 s=p[0]; return (0,int(s)) if s.isdigit() else (1,int(s[1:]) if s[1:].isdigit() else 999)
rows.sort(key=lambda x:(pk(x),x[1],x[2],x[3]))
for p,u,m,n,rname,card,item in rows:
 cid=canon_id(card); rec=state['cards'].setdefault(cid,{'kind':card['kind'],'cue':card['cue'],'attempts':0,'first_exact_total':0,'regenerated_exact_total':0,'consecutive_first_exact':0,'mastered':False,'mastered_at_pass':None,'history':[]})
 first=bool(item.get('first_exact'));regen=item.get('regenerated_exact') is True;rec['attempts']+=1;rec['first_exact_total']+=int(first);rec['regenerated_exact_total']+=int(regen);rec['consecutive_first_exact']=rec['consecutive_first_exact']+1 if first else 0
 if rec['consecutive_first_exact']>=2 and not rec['mastered']:rec['mastered']=True;rec['mastered_at_pass']=p
 rec['history'].append({'pass':p,'unit':u,'micro':m,'card':n,'first_exact':first,'regenerated_exact':regen,'result':rname})
state['processed_results']=sorted({x[4] for x in rows});state['updated_at']=datetime.now(timezone.utc).isoformat();atomic(OUT,state)
cs=list(state['cards'].values());print(json.dumps({'canonical_cards':len(cs),'mastered':sum(c['mastered'] for c in cs),'not_mastered':sum(not c['mastered'] for c in cs),'first_exact_attempts':sum(c['first_exact_total'] for c in cs),'regenerated_exact_attempts':sum(c['regenerated_exact_total'] for c in cs),'processed_results':len(state['processed_results'])},indent=2))
