#!/usr/bin/env python3
import re,json,glob,os,subprocess,collections
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT); head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
files=sorted(set(glob.glob('internal/droplets/*.go')+glob.glob('internal/provisioning/**/*.go',recursive=True)+glob.glob('internal/worker/**/*.go',recursive=True)+glob.glob('internal/scheduler/**/*.go',recursive=True)+glob.glob('internal/app/*lifecycle*.go')+glob.glob('internal/app/*deploy*.go')+glob.glob('internal/app/*recovery*.go')))
states=collections.defaultdict(list); transitions=[]; intervals=[]
state_re=re.compile(r'\b(?:state|State)\s*(?:==|!=|=|:|IN\s*\()?\s*["\']([A-Z][A-Z0-9_]+)["\']')
quoted=re.compile(r'["\']([A-Z][A-Z0-9_]{2,})["\']')
for p in files:
 text=open(p,encoding='utf-8').read(); lines=text.splitlines()
 for i,l in enumerate(lines,1):
  for st in quoted.findall(l):
   if any(k in st for k in ['READY','FAILED','PENDING','CREAT','INSTALL','PANEL','PROVISION','DELETE','RETRY','WAIT','BOOT','SSH','RUNNING','COMPLETE']): states[st].append({'file':p,'line':i,'text':l.strip()[:400]})
  if re.search(r'(?i)UPDATE\s+\w+\s+SET\s+state|state\s*=|State:',l):
   vals=quoted.findall(l)
   if vals: transitions.append({'file':p,'line':i,'states':vals,'text':l.strip()[:500]})
  if 'time.NewTicker' in l or 'time.NewTimer' in l or 'time.Sleep' in l or 'time.After' in l: intervals.append({'file':p,'line':i,'text':l.strip()[:400]})
out={'schema_version':1,'source_commit':head,'files':files,'states':dict(sorted(states.items())),'transition_evidence':transitions,'timing_evidence':intervals,'note':'Static evidence index. Transition direction/ownership must be confirmed from exact source before mutation.'}
json.dump(out,open('docs/LIFECYCLE_STATE_BRAIN.json','w',encoding='utf-8'),ensure_ascii=False,indent=2); open('docs/LIFECYCLE_STATE_BRAIN.json','a').write('\n')
md=['# Lifecycle State-Machine Brain','',f'Commit `{head}`. Indexed **{len(files)} lifecycle/provisioning/worker files**, **{len(states)} state-like constants**, **{len(transitions)} transition evidence locations**, **{len(intervals)} timing/retry locations**.','', 'This is static evidence, not a formally proven state machine. Before changing a transition, inspect its exact source, owner and persistence behavior.','', '## State-like values']+[f'- `{k}` — {len(v)} references' for k,v in sorted(states.items())]+['','## Ownership areas','- Compute lifecycle/reconciliation: `internal/droplets`','- Provisioning/install execution: `internal/provisioning`','- Worker ownership: `internal/worker`, `cmd/worker`','- Scheduling: `internal/scheduler`, `internal/app/scheduler.go`','- Deployment/recovery orchestration: `internal/app`']
open('docs/LIFECYCLE_STATE_BRAIN.md','w',encoding='utf-8').write('\n'.join(md)+'\n'); print('lifecycle files',len(files),'states',len(states),'transitions',len(transitions),'timers',len(intervals))
