#!/usr/bin/env python3
import json,sys,os
if len(sys.argv)!=2:raise SystemExit('usage: grade-fresh-session-exam.py ANSWERS.json')
key=json.load(open('docs/FRESH_SESSION_EXAM_KEY.json',encoding='utf-8'))['keys']; ans=json.load(open(sys.argv[1],encoding='utf-8')); answers=ans.get('answers',ans)
# Exact structural equality by design: fresh session should emit answer object from evidence.
rows=[]
for i,k in key.items():
 got=answers.get(i) if isinstance(answers,dict) else None; rows.append({'exam_id':int(i),'pass':got==k})
score=sum(x['pass'] for x in rows);print(f'FRESH SESSION EXAM {score}/{len(rows)}')
for x in rows:
 if not x['pass']:print('FAIL',x['exam_id'])
sys.exit(0 if score==len(rows) else 1)
