#!/usr/bin/env python3
import json,os,re,subprocess,collections
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT)
head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
S=json.load(open('docs/FULL_SOURCE_KNOWLEDGE.json',encoding='utf-8')); M=json.load(open('docs/SEMANTIC_KNOWLEDGE.json',encoding='utf-8')); Y=json.load(open('docs/SYMBOL_INDEX.json',encoding='utf-8'))
source={f['path']:f for f in S['files']}; route_by_handler=collections.defaultdict(list)
for r in Y['routes']:route_by_handler[r['handler']].append({'method':r['method'],'path':r['path']})
items=[]
for s in M['symbols']:
 if s['kind'] not in ('func','method'):continue
 f=source.get(s['file']);
 if not f:continue
 body='\n'.join(x['text'] for x in f['lines'][s['start_line']-1:s['end_line']])
 sig=f['lines'][s['start_line']-1]['text'].strip() if s['start_line']<=f['line_count'] else ''
 params=''; returns=''
 mm=re.search(r'func\s+(?:\([^)]*\)\s*)?'+re.escape(s['symbol'])+r'\s*\((.*?)\)\s*(.*?)(?:\{|$)',sig)
 if mm:params=mm.group(1).strip();returns=mm.group(2).strip()
 reads=sorted(set(re.findall(r'(?i)\b(?:FROM|JOIN)\s+([a-z_][a-z0-9_]*)',body)))
 writes=sorted(set(re.findall(r'(?i)\b(?:UPDATE|INSERT\s+INTO|DELETE\s+FROM)\s+([a-z_][a-z0-9_]*)',body)))
 errors=[]
 for line in body.splitlines():
  if 'return' in line and ('err' in line.lower() or 'Error' in line or 'error' in line):errors.append(line.strip()[:300])
 effects=[]
 if writes:effects.append('database_write')
 if re.search(r'http\.(?:NewRequest|Get|Post)|\.Do\(',body):effects.append('http_call')
 if re.search(r'ExecContext|QueryContext|QueryRowContext',body):effects.append('database_io')
 if re.search(r'os\.Exec|exec\.Command',body):effects.append('process_exec')
 if 'go func' in body:effects.append('goroutine')
 if 'time.NewTicker' in body:effects.append('ticker')
 item={'name':s['symbol'],'kind':s['kind'],'receiver':s.get('receiver',''),'package':s['package'],'file':s['file'],'start_line':s['start_line'],'end_line':s['end_line'],'signature':sig,'parameters_text':params,'returns_text':returns,'routes':route_by_handler.get(s['symbol'],[]),'calls':s.get('calls',[]),'candidate_caller_files':s.get('candidate_caller_files',[]),'db_reads':reads,'db_writes':writes,'side_effect_classes':effects,'error_return_evidence':errors[:12]}
 # grounded responsibility label from structural evidence, not invented prose
 role=[]
 if item['routes']:role.append('HTTP handler')
 if writes:role.append('persists/mutates DB state')
 if reads:role.append('reads DB state')
 if 'http_call' in effects:role.append('performs HTTP/API call')
 if not role:role.append('internal computation/orchestration')
 item['structural_responsibility']='; '.join(role)
 items.append(item)
out={'schema_version':1,'source_commit':head,'function_count':len(items),'methodology':'commit-pinned static behavior evidence; responsibility labels are structural, not semantic guesses','functions':items}
json.dump(out,open('docs/FUNCTION_BEHAVIOR_BRAIN.json','w'),ensure_ascii=False,separators=(',',':'));open('docs/FUNCTION_BEHAVIOR_BRAIN.json','a').write('\n')
# Summaries by package
pk=collections.Counter(x['package'] for x in items); routed=sum(bool(x['routes']) for x in items); dbw=sum(bool(x['db_writes']) for x in items); dbr=sum(bool(x['db_reads']) for x in items)
md=['# Function / Behavior Brain','',f'Commit `{head}` — **{len(items)} functions/methods** indexed.','',f'- HTTP handlers: **{routed}**','- Functions/methods with DB reads: **%d**'%dbr,'- Functions/methods with DB writes: **%d**'%dbw,'', 'For each function this brain stores exact source range/signature, parameter/return text, bound routes, lexical calls/candidate callers, DB reads/writes, structural side-effect classes and error-return evidence. It deliberately avoids inventing business meaning that static evidence cannot prove.','', '## Package coverage']+[f'- `{k}` — {v}' for k,v in sorted(pk.items())]
open('docs/FUNCTION_BEHAVIOR_BRAIN.md','w').write('\n'.join(md)+'\n')
print('function brain',len(items),'http',routed,'db-read',dbr,'db-write',dbw,'commit',head[:12])
