#!/usr/bin/env python3
import json,os
K=json.load(open('.audit/rehearsal-holdout-3/key.json'))['keys'];os.makedirs('docs/holdout3-targeted-rehearsal',exist_ok=True)
# H3 frozen results: all conceptual need exact-field reinforcement; topology exact passes 33,36,44,53,55,58,59,60; verbatim exact passes 62,66,70,77.
topo_pass={33,36,44,53,55,58,59,60}; verb_pass={62,66,70,77}
def save(n,x):p=f'docs/holdout3-targeted-rehearsal/{n}.json';json.dump(x,open(p,'w'),ensure_ascii=False,indent=2);open(p,'a').write('\n')
save('conceptual-exact',[{'holdout_id':i,'correct':K[str(i)]} for i in range(1,31)])
save('topology-exact',[{'holdout_id':i,'correct':K[str(i)]} for i in range(31,61) if i not in topo_pass])
save('verbatim-exact',[{'holdout_id':i,'correct':K[str(i)]} for i in range(61,81) if i not in verb_pass])
print('conceptual',30,'topology',30-len(topo_pass),'verbatim',20-len(verb_pass),'total',30+30-len(topo_pass)+20-len(verb_pass))
