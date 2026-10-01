#!/usr/bin/env python3
import os,re,json,glob,subprocess
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT); head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
files=sorted(set(['cmd/worker/main.go','cmd/api/main.go']+glob.glob('internal/worker/**/*.go',recursive=True)+glob.glob('internal/scheduler/**/*.go',recursive=True)+glob.glob('internal/app/*.go')))
events=[]
for p in files:
 if not os.path.exists(p):continue
 for i,l in enumerate(open(p,encoding='utf-8'),1):
  typ=None
  if re.search(r'\bgo\s+(?:func\s*\(|[A-Za-z_])',l):typ='goroutine_start'
  if 'time.NewTicker' in l:typ='ticker'
  if 'time.NewTimer' in l:typ='timer'
  if 'time.Sleep' in l:typ='sleep'
  if 'time.After' in l:typ='after'
  if typ: events.append({'type':typ,'file':p,'line':i,'text':l.strip()[:500]})
services={}
for s in ['digital-ocean-bot-api','digital-ocean-bot-worker']:
 q=subprocess.run(['systemctl','show',s,'-p','ActiveState','-p','SubState','-p','MainPID','--no-pager'],text=True,capture_output=True); services[s]=q.stdout.strip().splitlines()
out={'schema_version':1,'source_commit':head,'files':files,'runtime_events':events,'services':services,'note':'Static concurrency/cadence evidence plus live service state; exact ownership/control flow must be confirmed in source.'}
json.dump(out,open('docs/RUNTIME_WORKER_BRAIN.json','w'),indent=2);open('docs/RUNTIME_WORKER_BRAIN.json','a').write('\n')
print('runtime-worker files',len(files),'events',len(events))
