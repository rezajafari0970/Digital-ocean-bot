#!/usr/bin/env python3
import os,re,json,glob,subprocess,collections
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT)
head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
files=sorted(glob.glob('cmd/**/*.go',recursive=True)+glob.glob('internal/**/*.go',recursive=True))
parsed={}; defs=collections.defaultdict(list); token_files=collections.defaultdict(set)
ident=re.compile(r'\b[A-Za-z_]\w*\b')
for p in files:
 text=open(p,encoding='utf-8').read(); lines=text.splitlines(); pkg=(re.search(r'(?m)^package\s+(\w+)',text) or [None,''])[1]; syms=[]
 for i,l in enumerate(lines,1):
  m=re.match(r'\s*func\s+(?:\(([^)]*)\)\s*)?([A-Za-z_]\w*)\s*\(',l)
  if m: syms.append({'kind':'method' if m.group(1) else 'func','name':m.group(2),'receiver':(m.group(1) or '').strip(),'line':i})
  m=re.match(r'\s*type\s+([A-Za-z_]\w*)\s+(struct|interface)\b',l)
  if m: syms.append({'kind':m.group(2),'name':m.group(1),'receiver':'','line':i})
 imports=sorted(set(re.findall(r'"(github\.com/rezajafari0970/Digital-ocean-bot/internal/[^" ]+)"',text)))
 parsed[p]={'package':pkg,'text':text,'lines':lines,'symbols':syms,'imports':imports}
 for s in syms: defs[s['name']].append((p,s))
 for tok in set(ident.findall(text)): token_files[tok].add(p)
entries=[]
for p,d in parsed.items():
 syms=d['symbols']
 for j,s in enumerate(syms):
  end=(syms[j+1]['line']-1) if j+1<len(syms) else len(d['lines']); body='\n'.join(d['lines'][s['line']-1:end]); bodytokens=set(ident.findall(body))
  calls=sorted(n for n in bodytokens if n!=s['name'] and n in defs and re.search(r'\b'+re.escape(n)+r'\s*\(',body))
  callers=sorted(x for x in token_files.get(s['name'],set()) if x!=p)
  tables=sorted(set(re.findall(r'(?i)\b(?:FROM|JOIN|INTO|UPDATE|TABLE)\s+([a-z_][a-z0-9_]*)',body)))
  endpoints=sorted(set(re.findall(r'/(?:api|share|vultr-browser)[A-Za-z0-9_/{}/.:-]*',body)))
  entries.append({'symbol':s['name'],'kind':s['kind'],'receiver':s['receiver'],'package':d['package'],'file':p,'start_line':s['line'],'end_line':end,'calls':calls,'candidate_caller_files':callers,'sql_tables':tables,'endpoint_literals':endpoints})
def role(p):
 for k,v in [('internal/adminapi/','HTTP/Admin API handlers'),('internal/providers/digitalocean/','DigitalOcean provider implementation'),('internal/providers/vultrconsole/','Vultr interactive browser/console integration'),('internal/providers/vultr/','Vultr provider implementation'),('internal/providers/','provider contracts/registry/common behavior'),('internal/panels/sanaei/','Sanaei/x-ui integration'),('internal/panels/','panel lifecycle/policy/inventory subsystem'),('internal/droplets/','compute lifecycle/reconciliation'),('internal/network/','network/proxy isolation and health'),('internal/accounts/','account identity/isolation'),('internal/app/','application orchestration/composition'),('cmd/api/','API process entrypoint'),('cmd/worker/','worker process entrypoint')]:
  if p.startswith(k): return v
 return 'project implementation/support'
file_index=[]
for p,d in parsed.items(): file_index.append({'file':p,'package':d['package'],'role':role(p),'imports':d['imports'],'symbols':[s['name'] for s in d['symbols']],'sql_tables':sorted(set(t for e in entries if e['file']==p for t in e['sql_tables']))})
out={'schema_version':1,'source_commit':head,'methodology':'single-pass token reverse index + lexical symbol-body analysis; caller links are candidates, not compiler-proven call graph','file_count':len(file_index),'symbol_count':len(entries),'files':file_index,'symbols':entries}
with open('docs/SEMANTIC_KNOWLEDGE.json','w',encoding='utf-8') as f: json.dump(out,f,ensure_ascii=False,separators=(',',':')); f.write('\n')
roles=collections.Counter(x['role'] for x in file_index)
md=['# Semantic Source Knowledge','',f'Commit-pinned semantic/navigation layer for **{len(file_index)} Go files** and **{len(entries)} symbols** at `{head}`.','', 'Records file roles, imports, symbol ranges, lexical calls, candidate caller files, SQL tables and endpoint literals. Candidate callers are navigation hints, not a compiler-proven call graph.','', '## Major package roles']+[f'- **{k}** — {v} files' for k,v in sorted(roles.items())]+['','## Retrieval workflow','1. Feature ownership: `FEATURE_FLOW_INDEX.md`.','2. Semantic relationships: `SEMANTIC_KNOWLEDGE.json`.','3. Exact definitions: `SYMBOL_INDEX.json`.','4. Exact source lines: `FULL_SOURCE_KNOWLEDGE.json` / `source-lookup.py`.','5. Verify snapshot commit/hash against current Git before treating it as current truth.']
open('docs/SEMANTIC_KNOWLEDGE.md','w',encoding='utf-8').write('\n'.join(md)+'\n')
print(f'semantic commit={head[:12]} files={len(file_index)} symbols={len(entries)} bytes={os.path.getsize("docs/SEMANTIC_KNOWLEDGE.json")}')
