#!/usr/bin/env python3
import json,os,sys
x=json.load(open('docs/BLIND_KNOWLEDGE_CHALLENGE.json',encoding='utf-8')); bad=[]
for q in x['challenges']:
 if not q.get('question') or not q.get('answer_key') or not q.get('evidence'):bad.append(q['id'])
cats=x.get('categories',{}); required=['route-chain','db-chain','state-chain','ui-chain','history-chain','test-chain','intent-chain','line-recall']
for c in required:
 if cats.get(c,0)<10:bad.append('category:'+c)
print('BLIND CHALLENGE',len(x['challenges']),'questions','PASS' if not bad else 'FAIL')
if bad:print('gaps',bad);sys.exit(1)
