#!/usr/bin/env python3
import json,sys,re,os
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT)
def load(p): return json.load(open(p,encoding='utf-8'))
source=load('docs/FULL_SOURCE_KNOWLEDGE.json'); sem=load('docs/SEMANTIC_KNOWLEDGE.json'); sym=load('docs/SYMBOL_INDEX.json'); schema=load('docs/SCHEMA_INDEX.json'); tests=load('docs/TEST_MAP.json'); behavior=load('docs/FUNCTION_BEHAVIOR_BRAIN.json'); columns=load('docs/COLUMN_BEHAVIOR_BRAIN.json'); test_behavior=load('docs/TEST_BEHAVIOR_BRAIN.json')
q=' '.join(sys.argv[1:]).strip()
if not q: raise SystemExit('usage: project-brain-query.py <file:line | symbol | endpoint | table | text>')
result={'query':q,'source_commit':source['source_commit'],'matches':[]}
# file:line[-end]
m=re.fullmatch(r'(.+?):(\d+)(?:-(\d+))?',q)
if m:
 f=next((x for x in source['files'] if x['path']==m.group(1)),None)
 if f:
  a=int(m.group(2)); b=int(m.group(3) or a); result['matches'].append({'kind':'line','file':f['path'],'sha256':f['sha256'],'range':[a,b],'lines':f['lines'][a-1:b]})
# endpoint exact/substring
for r in sym.get('routes',[]):
 if q.lower() in r.get('path','').lower() or q==r.get('handler'):
  result['matches'].append({'kind':'route','route':r})
# symbols exact first, then substring
for s in sem.get('symbols',[]):
 if s['symbol']==q or q.lower() in s['symbol'].lower(): result['matches'].append({'kind':'symbol','semantic':s})
# function behavior exact/substring
for b in behavior.get('functions',[]):
 if b['name']==q or q.lower() in b['name'].lower(): result['matches'].append({'kind':'function_behavior','behavior':b})
# table
for t in schema.get('tables',[]):
 if t['table']==q or q.lower() in t['table'].lower(): result['matches'].append({'kind':'table','schema':t})
# test behavior exact/substring lookup
for t in test_behavior.get('tests',[]):
 if t['test']==q or q.lower() in t['test'].lower(): result['matches'].append({'kind':'test_behavior','test':t})
# database column exact table.column or column-name lookup
for c in columns.get('columns', []):
    key = c['table'] + '.' + c['column']

    if (
        q.lower() == key.lower()
        or q.lower() == c['column'].lower()
    ):
        result['matches'].append(
            {
                'kind': 'column_behavior',
                'column': c,
            }
        )

# filename/path and source textual fallback, capped
for f in source['files']:
 if q.lower() in f['path'].lower(): result['matches'].append({'kind':'file','path':f['path'],'sha256':f['sha256'],'line_count':f['line_count'],'symbols':f.get('symbols',[])})
if len(result['matches'])<20 and len(q)>=4:
 rx=re.compile(re.escape(q),re.I)
 for f in source['files']:
  for x in f['lines']:
   if rx.search(x['text']):
    result['matches'].append({'kind':'text','file':f['path'],'line':x['n'],'text':x['text']})
    if len(result['matches'])>=40: break
  if len(result['matches'])>=40: break
result['matches']=result['matches'][:40]; print(json.dumps(result,ensure_ascii=False,indent=2))
