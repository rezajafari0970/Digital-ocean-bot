#!/usr/bin/env python3
import json,sys,os
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT)
def J(p):return json.load(open(p,encoding='utf-8'))
if len(sys.argv)<3:raise SystemExit('usage: project-knowledge-contract.py TYPE QUERY')
typ,q=sys.argv[1],sys.argv[2];out=None
if typ=='db':
 x=J('docs/COLUMN_BEHAVIOR_BRAIN.json');
 for c in x['columns']:
  if (c['table']+'.'+c['column']).lower()==q.lower():out={'added_by':c.get('added_by'),'readers':c.get('reader_functions',[]),'writers':c.get('writer_functions',[])};break
elif typ=='route':
 x=J('docs/SYMBOL_INDEX.json')
 # query METHOD|PATH
 m,p=q.split('|',1)
 for r in x['routes']:
  if r['method']==m and r['path']==p:
   h=r.get('handler_definition') or {};out={'handler':r['handler'],'file':h.get('file'),'line':h.get('line')};break
elif typ=='state':
 x=J('docs/STATE_TRANSITION_BRAIN.json')
 # exact owner function query
 for z in x['owners']:
  if z['owner_function']==q:out={'function':z['owner_function'],'file':z['file'],'line':z['start_line'],'writes':z['transition_write_evidence'][:2],'guards':z['guard_evidence'][:2]};break
elif typ=='ui':
 x=J('docs/FRONTEND_ACTION_STATE_BRAIN.json')
 for z in x['functions']:
  if z['name']==q:out={'function':z['name'],'file':z['file'],'line':z['start_line'],'dom':z['dom_selectors'][:4],'loading':z['loading_evidence'][:3],'errors':z['error_evidence'][:3]};break
if out is None:raise SystemExit('NOT_FOUND')
print(json.dumps(out,ensure_ascii=False,indent=2))
