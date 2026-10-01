#!/usr/bin/env python3
import json,os,subprocess,re
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
ledger=open('docs/HISTORICAL_DECISION_LEDGER.md',encoding='utf-8').read(); log=subprocess.check_output(['git','log','--all','--date=short','--pretty=format:%H\t%ad\t%s','-n','500'],text=True)
keywords=re.compile(r'(?i)fix|bug|race|stale|duplicate|retry|fallback|oom|expiry|visibility|proxy|vultr|websocket|novnc|capacity|installer|migration|session|reconcile|error')
commits=[]
for line in log.splitlines():
 a=line.split('\t',2)
 if len(a)==3 and keywords.search(a[2]):commits.append({'commit':a[0],'date':a[1],'subject':a[2]})
# Explicit durable incidents/regressions grounded in existing historical ledger/project history.
incidents=[
 {'id':'INC-001','area':'provisioning','failure':'duplicate/competing installer execution','lesson':'Long-running install work needs one controlled execution owner; in-process locks alone are insufficient across processes.','prevent':['single lifecycle owner','per-resource coordination','observable install diagnostics']},
 {'id':'INC-002','area':'proxy_network','failure':'shared account egress/proxy identity','lesson':'Account network identity must remain isolated; shared proxy state is a regression.','prevent':['per-account isolation','fail-closed required proxy']},
 {'id':'INC-003','area':'output','failure':'stale output visibility / approximate expiry','lesson':'Use persisted exact visibility/expiry semantics and apply them consistently to direct/share output.','prevent':['visible_until','canonical validity filter','avoid coarse TTL-only logic']},
 {'id':'INC-004','area':'output','failure':'high-frequency refresh causing memory pressure/OOM risk','lesson':'Keep Output fast but use lightweight/separated refresh rather than unbounded expensive refresh work.','prevent':['bounded refresh work','live API path','avoid SSH aggregation']},
 {'id':'INC-005','area':'providers','failure':'DigitalOcean-specific assumptions leaking into generic provider paths','lesson':'Generic lifecycle must not validate/catalog/reconcile with DO-only assumptions.','prevent':['provider-neutral contracts','provider-specific drivers']},
 {'id':'INC-006','area':'vultr','failure':'unknown Vultr capacity treated as hard blocker','lesson':'Unknown API limit is not the same as zero capacity; authoritative capacity observation is provider-specific.','prevent':['LimitKnown semantics','console capacity observation when needed']},
 {'id':'INC-007','area':'vultr_console','failure':'noVNC/WebSocket path mismatch / connecting state','lesson':'Reverse proxy must preserve the supported WebSocket upgrade path and interactive session authorization.','prevent':['websockify path compatibility','fresh console ticket/session','interactive challenge visibility']},
 {'id':'INC-008','area':'database','failure':'schema/source drift or migration mismatch','lesson':'Verify live migration level/checksums before schema-sensitive work; do not infer live schema solely from source migrations.','prevent':['schema_migrations checksums','LIVE_DB_BRAIN','migration verification']},
 {'id':'INC-009','area':'production','failure':'assuming Git HEAD equals deployed production','lesson':'Service health does not prove revision identity.','prevent':['/version stamp','artifact manifest hashes','production revision verifier']},
 {'id':'INC-010','area':'continuity','failure':'new chat reconstructing project from stale/partial history','lesson':'Use canonical repo plus commit-pinned knowledge brains and acceptance gates.','prevent':['NEW_CHAT_ENTRYPOINT','Project Brain','handoff acceptance gate']}
]
# attach relevant commits by subject keyword overlap
for x in incidents:
 terms=set(re.findall(r'[a-z0-9]+',(x['area']+' '+x['failure']+' '+x['lesson']).lower()))-{'the','and','or','to','a','is','as','in','with','must','not','use'}
 scored=[]
 for c in commits:
  sub=set(re.findall(r'[a-z0-9]+',c['subject'].lower())); score=len(terms&sub)
  if score:scored.append((score,c))
 x['related_commits']=[c for _,c in sorted(scored,key=lambda z:-z[0])[:12]]
out={'schema_version':1,'source_commit':head,'incident_count':len(incidents),'candidate_regression_commits_count':len(commits),'incidents':incidents,'candidate_regression_commits':commits,'source_note':'Incidents encode durable lessons already present in project history/decision ledger; commit links are lexical candidates and must be inspected before causal attribution.'}
json.dump(out,open('docs/INCIDENT_REGRESSION_BRAIN.json','w'),ensure_ascii=False,indent=2);open('docs/INCIDENT_REGRESSION_BRAIN.json','a').write('\n')
md=['# Historical Incident / Regression Brain','',f'Commit `{head}` — **{len(incidents)} durable incident/regression classes**, **{len(commits)} candidate regression-related commits** from recent history.','', 'This brain teaches a fresh session what previously went wrong, the durable lesson, and the guardrails that must not regress. Commit associations are lexical navigation candidates, not automatic proof of causality.','']
for x in incidents:
 md += [f'## `{x["id"]}` — {x["area"]}',f'- Failure: {x["failure"]}',f'- Lesson: {x["lesson"]}','- Prevent: '+', '.join(x['prevent']),'']
open('docs/INCIDENT_REGRESSION_BRAIN.md','w').write('\n'.join(md)+'\n');print('incident brain',len(incidents),'candidate commits',len(commits),'commit',head[:12])
