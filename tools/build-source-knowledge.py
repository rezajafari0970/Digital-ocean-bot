#!/usr/bin/env python3
import os,glob,json,hashlib,subprocess,re
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT)
INCLUDE=['cmd/**/*.go','internal/**/*.go','migrations/*.sql','web/static/*.js','web/static/*.html','web/static/*.css','deploy/*.sh','deploy/*.service','tools/*.py','tools/*.sh']
files=sorted(set(p for pat in INCLUDE for p in glob.glob(pat,recursive=True) if os.path.isfile(p)))
head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(); records=[]; total_lines=0
for p in files:
 raw=open(p,'rb').read(); text=raw.decode('utf-8','replace'); lines=text.splitlines(); total_lines+=len(lines)
 rec={'path':p,'sha256':hashlib.sha256(raw).hexdigest(),'bytes':len(raw),'line_count':len(lines),'lines':[{'n':i+1,'text':v} for i,v in enumerate(lines)]}
 if p.endswith('.go'):
  rec['symbols']=[]
  for i,v in enumerate(lines,1):
   m=re.match(r'\s*func\s+(?:\(([^)]*)\)\s*)?([A-Za-z_]\w*)\s*\(',v)
   if m: rec['symbols'].append({'kind':'method' if m.group(1) else 'func','name':m.group(2),'line':i})
   m=re.match(r'\s*type\s+([A-Za-z_]\w*)\s+(struct|interface)\b',v)
   if m: rec['symbols'].append({'kind':m.group(2),'name':m.group(1),'line':i})
 records.append(rec)
out={'schema_version':1,'source_commit':head,'file_count':len(records),'total_lines':total_lines,'files':records}
with open('docs/FULL_SOURCE_KNOWLEDGE.json','w',encoding='utf-8') as f: json.dump(out,f,ensure_ascii=False,separators=(',',':')); f.write('\n')
cat={'schema_version':1,'source_commit':head,'file_count':len(records),'total_lines':total_lines,'files':[{'path':r['path'],'sha256':r['sha256'],'bytes':r['bytes'],'line_count':r['line_count'],'symbols':r.get('symbols',[])} for r in records]}
with open('docs/SOURCE_CATALOG.json','w',encoding='utf-8') as f: json.dump(cat,f,ensure_ascii=False,indent=2); f.write('\n')
print(f'source knowledge commit={head[:12]} files={len(records)} lines={total_lines} bytes={os.path.getsize("docs/FULL_SOURCE_KNOWLEDGE.json")}')
