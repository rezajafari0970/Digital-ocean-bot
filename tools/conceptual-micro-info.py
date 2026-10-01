#!/usr/bin/env python3
import json,sys,os
os.chdir(os.path.abspath(os.path.join(os.path.dirname(__file__),'..')));x=json.load(open('docs/CONCEPTUAL_MICRO_REHEARSAL_MANIFEST.json'))['micro_units'];u=int(sys.argv[1]);m=int(sys.argv[2]);z=next((q for q in x if q['unit']==u and q['micro']==m),None)
if not z:raise SystemExit('NOT_FOUND')
print(json.dumps(z,separators=(',',':')))
