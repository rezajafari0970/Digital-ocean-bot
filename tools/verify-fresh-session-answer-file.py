#!/usr/bin/env python3
import json,sys,hashlib,os
if len(sys.argv)!=2:raise SystemExit('usage: verify-fresh-session-answer-file.py ANSWERS.json')
p=sys.argv[1];x=json.load(open(p,encoding='utf-8'));a=x.get('answers')
if not isinstance(a,dict):raise SystemExit('FAIL answers must be object')
expected={str(i) for i in range(1,41)}; got=set(a)
if got!=expected:raise SystemExit(f'FAIL ids missing={sorted(expected-got)} extra={sorted(got-expected)}')
h=hashlib.sha256(open(p,'rb').read()).hexdigest();print('ANSWER FILE STRUCTURE PASS count=40 sha256='+h)
