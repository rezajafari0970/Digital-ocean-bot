#!/usr/bin/env python3
import os, subprocess, sys
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT)
steps=[
 ('build',['go','build','./cmd/api','./cmd/worker']),
 ('tests',['go','test','./...']),
 ('knowledge_drift',['python3','tools/knowledge-drift-gate.py']),
 ('fresh_mastery',['python3','tools/fresh-chat-mastery-gate.py']),
 ('continuity',['python3','tools/continuity-finalization-gate.py']),
 ('production',['python3','tools/verify-production-revision.py']),
]
failed=[]
for name,cmd in steps:
 p=subprocess.run(cmd,text=True,capture_output=True)
 print(('PASS' if p.returncode==0 else 'FAIL'),name)
 if p.returncode:
  failed.append(name)
  if p.stdout: print(p.stdout[-2000:])
  if p.stderr: print(p.stderr[-1000:],file=sys.stderr)
print(f'FINAL DEVELOPMENT MASTERY {"100% VERIFIED" if not failed else "NOT VERIFIED"}')
if failed: print('failed='+','.join(failed))
sys.exit(1 if failed else 0)
