#!/usr/bin/env python3
import json,sys,os
p=os.path.join(os.path.dirname(__file__),'..','docs','FULL_SOURCE_KNOWLEDGE.json'); d=json.load(open(p,encoding='utf-8'))
if len(sys.argv)<3: raise SystemExit('usage: source-lookup.py FILE LINE [END_LINE]')
name=sys.argv[1]; start=int(sys.argv[2]); end=int(sys.argv[3]) if len(sys.argv)>3 else start
f=next((x for x in d['files'] if x['path']==name),None)
if not f: raise SystemExit('file not indexed: '+name)
if start<1 or end>f['line_count'] or end<start: raise SystemExit(f'line range outside 1..{f["line_count"]}')
print(f'commit={d["source_commit"]} file={name} sha256={f["sha256"]} lines={start}-{end}')
for x in f['lines'][start-1:end]: print(f'{x["n"]:6d} | {x["text"]}')
