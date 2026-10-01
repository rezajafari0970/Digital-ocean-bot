#!/usr/bin/env python3
import re,json,os,subprocess
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..'));os.chdir(ROOT);head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
path='web/static/app.js';lines=open(path,encoding='utf-8').read().splitlines(); text='\n'.join(lines)
# function ranges: declarations, async declarations, and window.foo assignments
starts=[]
for i,l in enumerate(lines,1):
 m=re.match(r'\s*(?:async\s+)?function\s+([A-Za-z_]\w*)\s*\(',l)
 if m:starts.append((i,m.group(1),'function'))
 m=re.match(r'\s*window\.([A-Za-z_]\w*)\s*=\s*(?:async\s*)?\(?',l)
 if m:starts.append((i,m.group(1),'window'))
starts=sorted(set(starts)); funcs=[]
for j,(st,name,kind) in enumerate(starts):
 en=starts[j+1][0]-1 if j+1<len(starts) else len(lines); body='\n'.join(lines[st-1:en])
 apis=sorted(set(re.findall(r'["\'`](/api/v1/[^"\'`?+ ]+)',body)))
 dom=sorted(set(re.findall(r'(?:getElementById|querySelector)\(["\'`]([^"\'`]+)',body)))
 classes=sorted(set(re.findall(r'classList\.(?:add|remove|toggle)\(["\'`]([^"\'`]+)',body)))
 storage=sorted(set(re.findall(r'(?:localStorage|sessionStorage)\.([A-Za-z_]\w*)',body)))
 calls=sorted(set(re.findall(r'\b([A-Za-z_]\w*)\s*\(',body))-{name,'if','for','switch','catch'})
 funcs.append({'name':name,'kind':kind,'file':path,'start_line':st,'end_line':en,'api_literals':apis,'dom_selectors':dom,'class_mutations':classes,'storage_ops':storage,'candidate_calls':calls[:150],'has_try':('try {' in body or 'try{' in body),'has_catch':('catch' in body),'loading_evidence':[x.strip()[:300] for x in body.splitlines() if re.search(r'loading|disabled|spinner|busy',x,re.I)][:20],'error_evidence':[x.strip()[:300] for x in body.splitlines() if re.search(r'error|failed|catch|alert\(',x,re.I)][:20]})
# data-action occurrences and containing/nearby context
actions=[]
for i,l in enumerate(lines,1):
 for a in re.findall(r'data-action=[\\"\']?([^\\"\' >+]+)',l):
  owner=next((f for f in funcs if f['start_line']<=i<=f['end_line']),None); actions.append({'action':a,'line':i,'owner_function':owner['name'] if owner else None,'context':l.strip()[:500]})
# click/change dispatch evidence
handlers=[]
for i,l in enumerate(lines,1):
 if re.search(r'addEventListener\(["\'](?:click|change|submit|input)',l):
  owner=next((f for f in funcs if f['start_line']<=i<=f['end_line']),None);handlers.append({'line':i,'owner_function':owner['name'] if owner else None,'text':l.strip()[:500]})
out={'schema_version':1,'source_commit':head,'file':path,'function_count':len(funcs),'action_count':len(actions),'event_handler_evidence_count':len(handlers),'functions':funcs,'actions':actions,'event_handlers':handlers}
json.dump(out,open('docs/FRONTEND_ACTION_STATE_BRAIN.json','w'),ensure_ascii=False,indent=2);open('docs/FRONTEND_ACTION_STATE_BRAIN.json','a').write('\n')
api=sum(bool(x['api_literals']) for x in funcs);dom=sum(bool(x['dom_selectors']) for x in funcs);err=sum(bool(x['error_evidence']) for x in funcs)
md=['# Frontend Action / State Brain','',f'Commit `{head}` — `app.js` functions/window handlers: **{len(funcs)}**, data-actions: **{len(actions)}**, event-handler evidence: **{len(handlers)}**.','',f'- Functions with API literals: **{api}**',f'- Functions with DOM selector evidence: **{dom}**',f'- Functions with error-handling evidence: **{err}**','', 'For each frontend function this brain records exact source range, API literals, DOM selectors, class mutations, browser storage operations, candidate calls, loading evidence and error evidence. Actions/events are linked to containing functions where static structure permits. Exact runtime behavior remains source/browser authoritative.']
open('docs/FRONTEND_ACTION_STATE_BRAIN.md','w').write('\n'.join(md)+'\n');print('frontend action brain funcs',len(funcs),'actions',len(actions),'events',len(handlers),'api-funcs',api,'commit',head[:12])
