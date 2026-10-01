#!/usr/bin/env python3
import argparse,json,os,tempfile,datetime
ROOT=os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
MAN=os.path.join(ROOT,'docs/SPACED_SOURCE_REHEARSAL_MICRO_MANIFEST.json')
STATE=os.path.join(ROOT,'.local','spaced-rehearsal-progress.json')

def load_manifest(): return json.load(open(MAN,encoding='utf-8'))
def sequence(m):
 out=[]
 for ph in m['phases']:
  for x in ph['micros']:
   out.append({'pass':str(ph['pass']),'unit':x['parent_unit'],'micro':x['micro'],'path':x['path'],'cards':x['cards']})
 return out

def atomic(obj):
 os.makedirs(os.path.dirname(STATE),exist_ok=True); fd,tmp=tempfile.mkstemp(dir=os.path.dirname(STATE),prefix='.spaced-',text=True)
 with os.fdopen(fd,'w') as f: json.dump(obj,f,indent=2);f.write('\n');f.flush();os.fsync(f.fileno())
 os.replace(tmp,STATE)
def key(x): return f"{x['pass']}:{x['unit']}:{x['micro']}"
def init(seq):
 # Preserve historical completion through Pass1 Unit23; Unit24+ unknown except user-reported safety events are not inferred here.
 st={'schema_version':1,'statuses':{},'cursor':None,'updated_at':None}
 for x in seq:
  if x['pass']=='1' and x['unit']<=23: st['statuses'][key(x)]='COMPLETE'
  else: break
 advance(st,seq);return st
def advance(st,seq):
 for x in seq:
  s=st['statuses'].get(key(x))
  if s not in ('COMPLETE','SAFETY_BLOCKED'):
   st['cursor']=x;break
 else: st['cursor']=None
 st['updated_at']=datetime.datetime.now(datetime.timezone.utc).isoformat();atomic(st)
def main():
 ap=argparse.ArgumentParser();sub=ap.add_subparsers(dest='cmd',required=True)
 sub.add_parser('init');sub.add_parser('next');sub.add_parser('summary')
 m=sub.add_parser('mark');m.add_argument('status',choices=['COMPLETE','RETRYABLE','SAFETY_BLOCKED']);m.add_argument('--pass',dest='p');m.add_argument('--unit',type=int);m.add_argument('--micro',type=int)
 a=ap.parse_args();seq=sequence(load_manifest())
 if a.cmd=='init':
  if os.path.exists(STATE): st=json.load(open(STATE))
  else: st=init(seq)
 elif os.path.exists(STATE): st=json.load(open(STATE))
 else: st=init(seq)
 if a.cmd=='mark':
  cur=st['cursor']; target=cur
  if a.p is not None: target=next((x for x in seq if x['pass']==a.p and x['unit']==a.unit and x['micro']==a.micro),None)
  if not target: raise SystemExit('target not found')
  # Safety blocks are terminal-for-training-run and advance. Retryable stays cursor.
  st['statuses'][key(target)]=a.status
  if a.status=='RETRYABLE': st['cursor']=target;st['updated_at']=datetime.datetime.now(datetime.timezone.utc).isoformat();atomic(st)
  else: advance(st,seq)
 if a.cmd in ('init','next','mark'):
  print(json.dumps({'cursor':st['cursor'],'status_counts':{z:list(st['statuses'].values()).count(z) for z in ['COMPLETE','RETRYABLE','SAFETY_BLOCKED']}},ensure_ascii=False))
 else:
  from collections import Counter
  print(json.dumps({'cursor':st['cursor'],'counts':Counter(st['statuses'].values()),'total_micros':len(seq),'updated_at':st['updated_at']},default=dict,ensure_ascii=False,indent=2))
if __name__=='__main__':main()
