#!/usr/bin/env python3
import json,os,subprocess,collections,re,hashlib
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
def J(p):return json.load(open(p,encoding='utf-8'))
S=J('docs/FULL_SOURCE_KNOWLEDGE.json'); M=J('docs/SEMANTIC_KNOWLEDGE.json'); F=J('docs/FUNCTION_BEHAVIOR_BRAIN.json'); Y=J('docs/SYMBOL_INDEX.json')
fb={(x['file'],x['name'],x['start_line']):x for x in F['functions']}; byfile=collections.defaultdict(list)
for s in M['symbols']:byfile[s['file']].append(s)
items=[]
for path,syms in sorted(byfile.items()):
 syms=sorted(syms,key=lambda x:x['start_line'])
 for i,s in enumerate(syms):
  b=fb.get((s['file'],s['symbol'],s['start_line']),{}); prev=syms[i-1]['symbol'] if i else None; nxt=syms[i+1]['symbol'] if i+1<len(syms) else None
  items.append({'file':path,'symbol':s['symbol'],'kind':s['kind'],'package':s['package'],'receiver':s.get('receiver',''),'start_line':s['start_line'],'end_line':s['end_line'],'signature':b.get('signature',''),'previous_symbol':prev,'next_symbol':nxt,'calls':s.get('calls',[])[:40],'candidate_caller_files':s.get('candidate_caller_files',[])[:30],'db_reads':b.get('db_reads',[]),'db_writes':b.get('db_writes',[]),'routes':b.get('routes',[]),'side_effects':b.get('side_effect_classes',[]),'structural_role':b.get('structural_responsibility',''),'error_evidence_count':len(b.get('error_return_evidence',[]))})
# exact source blocks for memory drills, 8 lines each; store hashes and text in tracked pack because source is already tracked/snapshotted.
blocks=[]
for f in S['files']:
 if not f['path'].endswith(('.go','.js','.sql')):continue
 lines=f['lines']
 for st in range(1,len(lines)+1,8):
  chunk=lines[st-1:min(st+7,len(lines))];txt='\n'.join(x['text'] for x in chunk);blocks.append({'file':f['path'],'start_line':st,'end_line':chunk[-1]['n'],'sha256':hashlib.sha256(txt.encode()).hexdigest(),'text':txt})
out={'schema_version':1,'source_commit':S['source_commit'],'generated_at_head':head,'symbol_memory_count':len(items),'source_block_count':len(blocks),'symbol_memory':items,'source_blocks':blocks}
json.dump(out,open('docs/SOURCE_INTERNALIZATION_BRAIN.json','w'),ensure_ascii=False,separators=(',',':'));open('docs/SOURCE_INTERNALIZATION_BRAIN.json','a').write('\n')
# topology pack: compact names meant for repeated active-context ingestion.
topo={'source_commit':S['source_commit'],'files':[],'routes':[{'method':r['method'],'path':r['path'],'handler':r['handler']} for r in Y['routes']]}
for path,syms in sorted(byfile.items()):topo['files'].append({'file':path,'symbols':[x['symbol'] for x in sorted(syms,key=lambda z:z['start_line'])]})
json.dump(topo,open('docs/SYMBOL_TOPOLOGY_MEMORY_PACK.json','w'),ensure_ascii=False,separators=(',',':'));open('docs/SYMBOL_TOPOLOGY_MEMORY_PACK.json','a').write('\n')
print('internalization symbols',len(items),'blocks',len(blocks),'files',len(topo['files']),'routes',len(topo['routes']))
