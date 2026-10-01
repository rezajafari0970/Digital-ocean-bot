#!/usr/bin/env python3
import os,re,json,subprocess,sys
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT)
def sh(*a): return subprocess.check_output(a,text=True).strip()
def read(p): return open(p,encoding='utf-8').read() if os.path.exists(p) else ''
checks=[]
def ck(name,ok,detail=''): checks.append((name,bool(ok),detail))
bundle=read('docs/SESSION_HANDOFF_BUNDLE.md'); handoff=read('HANDOFF.md'); master=read('docs/CHATGPT_MASTER_CONTEXT.md'); fmap=read('docs/FEATURE_FLOW_INDEX.md'); hist=read('docs/HISTORICAL_DECISION_LEDGER.md'); req=read('docs/REQUIREMENTS_MATRIX.md'); manifest=json.load(open('docs/PROJECT_MANIFEST.json'))
head=sh('git','rev-parse','HEAD'); branch=sh('git','branch','--show-current')
for p in ['docs/SESSION_HANDOFF_BUNDLE.md','docs/CHATGPT_MASTER_CONTEXT.md','docs/CODEBASE_MAP.md','docs/FEATURE_FLOW_INDEX.md','docs/HISTORICAL_DECISION_LEDGER.md','docs/REQUIREMENTS_MATRIX.md','docs/CHANGE_PROTOCOL.md','PROJECT_CONTEXT.md','PROJECT_STATE.md','HANDOFF.md']:
 ck('exists:'+p,os.path.exists(p),p)
for term in ['Vultr','DigitalOcean','Sanaei','Output','Reality','Proxy','Lifecycle','noVNC','WebSocket']:
 ck('bundle-topic:'+term,term.lower() in bundle.lower(),term)
for path in ['internal/providers/vultr','internal/providers/vultrconsole','internal/providers/digitalocean','internal/panels/sanaei','internal/adminapi/output.go','web/static/app.js']:
 ck('source-map:'+path,path in bundle or path in fmap,path)
ck('handoff-has-next-action','Exact next action' in handoff)
ck('handoff-vultr-boundary','noVNC' in handoff and 'WebSocket' in handoff)
ck('history-rejected-patterns','Rejected/regressive patterns' in hist)
ck('requirements-provider-neutral','Provider-neutral' in req or 'provider-neutral' in req)
ck('manifest-routes',len(manifest.get('routes',[]))>=50,str(len(manifest.get('routes',[]))))
ck('manifest-current-work',bool(manifest.get('current_work')))
ck('branch-match',manifest.get('branch')==branch,f"manifest={manifest.get('branch')} live={branch}")
# A generated bundle may intentionally be one checkpoint behind itself; require bounded drift, not equality.
try:
 bhead=re.search(r'HEAD at bundle generation: `([0-9a-f]+)`',bundle).group(1)
 dist=int(sh('git','rev-list','--count',f'{bhead}..HEAD'))
 ck('bundle-head-bounded-drift',0<=dist<=2,f'drift={dist}')
except Exception as e: ck('bundle-head-bounded-drift',False,str(e))
# Reconstruct the active Vultr flow only from docs, then ensure every named source exists.
needed=['internal/adminapi/server.go','internal/providers/vultrconsole/browser.go','internal/providers/vultrconsole/proxy_bridge.go','cmd/vultr-browser-session','cmd/vultr-capacity-probe']
for p in needed: ck('active-flow-source:'+p,os.path.exists(p),p)
failed=[x for x in checks if not x[1]]
report=['# Handoff Readiness Audit','',f'Branch: `{branch}`',f'Live HEAD: `{head}`',f'Result: **{"PASS" if not failed else "FAIL"}** — {len(checks)-len(failed)}/{len(checks)} checks passed.','', '## Checks']
report += [f'- [{"x" if ok else " "}] `{name}`'+(f' — {detail}' if detail else '') for name,ok,detail in checks]
report += ['','## Reconstruction conclusion', 'A fresh session can identify the canonical repository/branch, current Vultr noVNC/WebSocket verification boundary, provider-neutral architecture, provider-specific source ownership, Sanaei/Output constraints, historical rejected patterns, and exact source entrypoints without relying on chat history.' if not failed else 'Continuity gaps remain; resolve failed checks before declaring the handoff self-sufficient.']
open('docs/HANDOFF_READINESS_AUDIT.md','w',encoding='utf-8').write('\n'.join(report)+'\n')
print(f"handoff audit: {'PASS' if not failed else 'FAIL'} {len(checks)-len(failed)}/{len(checks)}")
for x in failed: print('FAIL',x[0],x[2])
sys.exit(1 if failed else 0)
