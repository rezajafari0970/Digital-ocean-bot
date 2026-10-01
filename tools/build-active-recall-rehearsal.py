#!/usr/bin/env python3
import json,os,random,hashlib
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);M=json.load(open('docs/SOURCE_MEMORY_PACK_MANIFEST.json'));rng=random.Random(20261001);os.makedirs('docs/source-memory-rehearsal',exist_ok=True);manifest=[]
# Build question-only rehearsal sheets plus separate correction sheets. Training may use both in sequence; holdout exam remains separate.
def save(name,obj):
 p='docs/source-memory-rehearsal/'+name;data=json.dumps(obj,ensure_ascii=False,indent=2)+'\n';open(p,'w').write(data);return p,hashlib.sha256(data.encode()).hexdigest()
# topology packs: prompt bidirectionally on a sample from every file and all routes in pack 1.
for idx,p in enumerate(M['topology_packs'],1):
 d=json.load(open(p['path']));q=[];a={};n=0
 for f in d['files']:
  sy=f['symbols'];
  if not sy:continue
  for sym in rng.sample(sy,min(2,len(sy))):
   n+=1;q.append({'id':n,'prompt':f"Which file contains `{sym}`?"});a[str(n)]=f['file']
  if len(sy)>=2:
   j=rng.randrange(len(sy)-1);n+=1;q.append({'id':n,'prompt':f"In `{f['file']}`, what immediately follows `{sy[j]}`?"});a[str(n)]=sy[j+1]
 for r in d.get('routes',[]):n+=1;q.append({'id':n,'prompt':f"Handler for `{r['method']} {r['path']}`?"});a[str(n)]=r['handler']
 qp,qh=save(f'topology-{idx:03d}-recall.json',{'questions':q});cp,ch=save(f'topology-{idx:03d}-correction.json',{'answers':a});manifest.append({'kind':'topology','pack':p['path'],'recall':qp,'correction':cp,'questions':n})
# conceptual: symbol->core facts and reverse file/signature cues.
for idx,p in enumerate(M['conceptual_packs'],1):
 d=json.load(open(p['path']));q=[];a={};n=0
 for x in d:
  n+=1;q.append({'id':n,'prompt':f"Recall `{x['symbol']}` in `{x['file']}`: package, previous/next, calls, DB reads/writes, routes."});a[str(n)]={'package':x['package'],'previous_symbol':x['previous_symbol'],'next_symbol':x['next_symbol'],'calls':x['calls'],'db_reads':x['db_reads'],'db_writes':x['db_writes'],'routes':x['routes']}
 qp,qh=save(f'concept-{idx:03d}-recall.json',{'questions':q});cp,ch=save(f'concept-{idx:03d}-correction.json',{'answers':a});manifest.append({'kind':'conceptual','pack':p['path'],'recall':qp,'correction':cp,'questions':n})
# verbatim cohort 1 only: first-line completion and exact whole-block reproduction.
for idx,p in enumerate(M['verbatim_packs'][:5],1):
 d=json.load(open(p['path']));q=[];a={};n=0
 for x in d:
  lines=x['text'].split('\n');
  if not x['text'].strip():continue
  n+=1;q.append({'id':n,'prompt':f"Reproduce exactly `{x['file']}` lines {x['start_line']}-{x['end_line']}."});a[str(n)]=x['text']
  if len(lines)>1:
   n+=1;q.append({'id':n,'prompt':f"Continue exactly after first line `{lines[0]}` in `{x['file']}` block {x['start_line']}-{x['end_line']}."});a[str(n)]='\n'.join(lines[1:])
 qp,qh=save(f'verbatim-{idx:03d}-recall.json',{'questions':q});cp,ch=save(f'verbatim-{idx:03d}-correction.json',{'answers':a});manifest.append({'kind':'verbatim','pack':p['path'],'recall':qp,'correction':cp,'questions':n})
json.dump({'schema_version':1,'items':manifest},open('docs/ACTIVE_RECALL_REHEARSAL_MANIFEST.json','w'),indent=2);open('docs/ACTIVE_RECALL_REHEARSAL_MANIFEST.json','a').write('\n');print('rehearsal units',len(manifest),'questions',sum(x['questions'] for x in manifest))
