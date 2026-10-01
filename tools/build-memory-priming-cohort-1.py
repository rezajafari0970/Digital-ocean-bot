#!/usr/bin/env python3
import json,os,hashlib,random
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);M=json.load(open('docs/SOURCE_MEMORY_PACK_MANIFEST.json'));X=json.load(open('docs/SOURCE_INTERNALIZATION_BRAIN.json'))
# Cohort 1: all topology + all conceptual + first 5 verbatim packs (250 blocks max).
vp=M['verbatim_packs'][:5]; primed=[]
for p in vp:primed += json.load(open(p['path'],encoding='utf-8'))
primed_ids={(x['file'],x['start_line']) for x in primed}; hold=[x for x in X['source_blocks'] if (x['file'],x['start_line']) in primed_ids and x['end_line']-x['start_line']+1>=5]
rng=random.Random(int(X['source_commit'][12:24],16)); verb=rng.sample(hold,min(20,len(hold)))
# conceptual/topology holdout questions are selected from material present in priming packs, but question identities are not exposed during priming.
syms=X['symbol_memory']; concept=rng.sample([x for x in syms if x['calls'] or x['db_reads'] or x['routes'] or x['side_effects']],30)
topo=json.load(open('docs/SYMBOL_TOPOLOGY_MEMORY_PACK.json')); tf=rng.sample([x for x in topo['files'] if len(x['symbols'])>=2],20); rr=rng.sample(topo['routes'],10)
qs=[];keys={}
def add(metric,prompt,key):i=len(qs)+1;qs.append({'id':i,'metric':metric,'prompt':prompt});keys[str(i)]=key
for x in concept:add('conceptual',f"Closed-book: describe `{x['symbol']}` in `{x['file']}`: package, signature, previous/next symbol, calls, DB reads/writes, routes, side effects.",{'package':x['package'],'signature':x['signature'],'previous_symbol':x['previous_symbol'],'next_symbol':x['next_symbol'],'calls':x['calls'],'db_reads':x['db_reads'],'db_writes':x['db_writes'],'routes':x['routes'],'side_effects':x['side_effects']})
for x in tf:add('topology',f"Closed-book: list ordered symbols in `{x['file']}`.",{'symbols':x['symbols']})
for x in rr:add('topology',f"Closed-book: handler for `{x['method']} {x['path']}`?",{'handler':x['handler']})
for x in verb:add('verbatim',f"Closed-book: reproduce exactly lines {x['start_line']}-{x['end_line']} of `{x['file']}`.",{'text':x['text']})
os.makedirs('.audit/memory-cohort-1',exist_ok=True);kp='.audit/memory-cohort-1/key.json';json.dump({'keys':keys},open(kp,'w'),ensure_ascii=False,indent=2);open(kp,'a').write('\n')
public={'schema_version':1,'source_commit':X['source_commit'],'cohort':{'conceptual_packs':[p['path'] for p in M['conceptual_packs']],'topology_packs':[p['path'] for p in M['topology_packs']],'verbatim_packs':[p['path'] for p in vp],'verbatim_blocks_primed':len(primed),'repository_blocks_total':X['source_block_count'],'verbatim_repository_coverage_percent':round(100*len(primed)/X['source_block_count'],2)},'questions':qs};json.dump(public,open('docs/MEMORY_PRIMING_COHORT_1_EXAM.json','w'),ensure_ascii=False,indent=2);open('docs/MEMORY_PRIMING_COHORT_1_EXAM.json','a').write('\n')
# ordered ingestion manifest only, no answers/questions mixed into training.
train={'order':[p['path'] for p in M['topology_packs']]+[p['path'] for p in M['conceptual_packs']]+[p['path'] for p in vp],'closed_book_exam':'docs/MEMORY_PRIMING_COHORT_1_EXAM.json'};json.dump(train,open('docs/MEMORY_PRIMING_COHORT_1_PLAN.json','w'),indent=2);open('docs/MEMORY_PRIMING_COHORT_1_PLAN.json','a').write('\n')
print('cohort1 questions',len(qs),'primed verbatim blocks',len(primed),'coverage',public['cohort']['verbatim_repository_coverage_percent'],'keysha',hashlib.sha256(open(kp,'rb').read()).hexdigest())
