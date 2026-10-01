#!/usr/bin/env python3
import json,os,collections
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT)
def load(p): return json.load(open(p,encoding='utf-8'))
req=load('docs/INTENT_REQUIREMENTS_BRAIN.json'); sem=load('docs/SEMANTIC_KNOWLEDGE.json'); routes=load('docs/SYMBOL_INDEX.json')['routes']; schema=load('docs/SCHEMA_INDEX.json'); tests=load('docs/TEST_MAP.json')
nodes={}; edges=[]
def node(i,t,label,**meta): nodes[i]={'id':i,'type':t,'label':label,**meta}
def edge(a,b,rel): edges.append({'from':a,'to':b,'relation':rel})
for r in req['requirements']:
 rid='req:'+r['id']; node(rid,'requirement',r['id'],area=r['area'],requirement=r['requirement'])
 for p in r.get('code',[]): fid='path:'+p; node(fid,'path',p); edge(rid,fid,'implemented_by')
for x in sem['files']: node('file:'+x['file'],'file',x['file'],package=x['package'],role=x['role'])
for s in sem['symbols']:
 sid='sym:'+s['file']+':'+str(s['start_line'])+':'+s['symbol']; node(sid,'symbol',s['symbol'],file=s['file'],line=s['start_line'],package=s['package']); edge('file:'+s['file'],sid,'defines')
 for t in s.get('sql_tables',[]): edge(sid,'table:'+t,'touches')
for r in routes:
 rid='route:'+r['method']+' '+r['path']; node(rid,'route',r['method']+' '+r['path'],handler=r['handler']); h=r.get('handler_definition')
 if h: edge(rid,'sym:'+h['file']+':'+str(h['line'])+':'+h['name'],'handled_by')
for t in schema['tables']: node('table:'+t['table'],'table',t['table'],created_by=t.get('created_by'),code_usage=t.get('code_usage',[]))
for feat,v in tests['features'].items():
 q='testfeature:'+feat; node(q,'test_feature',feat,command=v['command']);
 for f in v['files']: node('testfile:'+f,'test_file',f); edge(q,'testfile:'+f,'verified_by')
# requirement path prefixes -> concrete files
for r in req['requirements']:
 for p in r.get('code',[]):
  for f in sem['files']:
   if f['file']==p or f['file'].startswith(p.rstrip('/')+'/'): edge('req:'+r['id'],'file:'+f['file'],'covers_file')
out={'schema_version':1,'source_commit':sem['source_commit'],'node_count':len(nodes),'edge_count':len(edges),'nodes':list(nodes.values()),'edges':edges}
json.dump(out,open('docs/IMPACT_GRAPH.json','w',encoding='utf-8'),ensure_ascii=False,separators=(',',':')); open('docs/IMPACT_GRAPH.json','a').write('\n')
print('impact graph nodes',len(nodes),'edges',len(edges),'requirements',len(req['requirements']))
