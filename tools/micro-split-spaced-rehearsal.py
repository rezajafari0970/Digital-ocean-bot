#!/usr/bin/env python3
import json,os
m=json.load(open('docs/SPACED_SOURCE_REHEARSAL_MANIFEST.json'));out='docs/spaced-source-rehearsal-micro';os.makedirs(out,exist_ok=True);phases=[];total=0
for phase in m['passes']:
 micros=[];msize=2 if phase['domain']=='verbatim' else 3
 for ui,path in enumerate(phase['units'],1):
  cards=json.load(open(path))
  for mi,s in enumerate(range(0,len(cards),msize),1):
   p=f"{out}/pass-{phase['pass']}-unit-{ui:03d}-micro-{mi:02d}.json";json.dump(cards[s:s+msize],open(p,'w'),ensure_ascii=False,separators=(',',':'));open(p,'a').write('\n');micros.append({'parent_unit':ui,'micro':mi,'path':p,'cards':len(cards[s:s+msize])});total+=1
 phases.append({'pass':phase['pass'],'domain':phase['domain'],'parent_units':len(phase['units']),'micros':micros})
json.dump({'schema_version':1,'source_commit':m['source_commit'],'policy':'micro-resumable; conceptual/topology <=3 cards; verbatim <=2 cards','phases':phases},open('docs/SPACED_SOURCE_REHEARSAL_MICRO_MANIFEST.json','w'),ensure_ascii=False,indent=2);open('docs/SPACED_SOURCE_REHEARSAL_MICRO_MANIFEST.json','a').write('\n');print('micro_total',total);[print('pass',p['pass'],'micros',len(p['micros'])) for p in phases]
