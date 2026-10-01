#!/usr/bin/env python3
import re,json,os,subprocess
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT); head=subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip()
js=open('web/static/app.js',encoding='utf-8').read(); html=open('web/static/index.html',encoding='utf-8').read(); css=open('web/static/app.css',encoding='utf-8').read()
api_calls=sorted(set(re.findall(r"['\"](/api/v1/[^'\"`?+ ]+)",js)))
actions=sorted(set(re.findall(r'data-action=[\\"\']?([^\\"\' >+]+)',js)))
windows=sorted(set(re.findall(r'window\.([A-Za-z_]\w*)\s*=',js)))
ids=sorted(set(re.findall(r'id=["\']([^"\']+)',html)))
classes=sorted(set(c for grp in re.findall(r'class=["\']([^"\']+)',html) for c in grp.split()))
pages=[]
m=re.search(r'const pages=\[(.*?)\];',js,re.S)
if m: pages=re.findall(r"\['([^']+)','([^']+)'\]",m.group(1))
# Map action literal to nearby handler dispatch snippet.
dispatch={}
for a in actions:
 hits=[m.start() for m in re.finditer(re.escape(a),js)]
 dispatch[a]=[js[max(0,x-180):min(len(js),x+260)].replace('\n',' ') for x in hits[:3]]
out={'schema_version':1,'source_commit':head,'pages':[{'id':a,'label':b} for a,b in pages],'api_paths':api_calls,'actions':actions,'window_functions':windows,'html_ids':ids,'html_classes':classes,'css_selector_count':len(re.findall(r'(?m)(?:^|})\s*([^@][^{]+)\{',css)),'action_context':dispatch}
json.dump(out,open('docs/FRONTEND_BRAIN.json','w',encoding='utf-8'),ensure_ascii=False,indent=2); open('docs/FRONTEND_BRAIN.json','a').write('\n')
md=['# Frontend / UI Brain','',f'Commit `{head}`. Pages: **{len(pages)}**, API path literals: **{len(api_calls)}**, UI actions: **{len(actions)}**, window-level handlers/functions: **{len(windows)}**.','', '## Pages']+[f'- `{a}` — {b}' for a,b in pages]+['','## API paths']+[f'- `{x}`' for x in api_calls]+['','## UI actions']+[f'- `{x}`' for x in actions]
open('docs/FRONTEND_BRAIN.md','w',encoding='utf-8').write('\n'.join(md)+'\n'); print('frontend',len(pages),len(api_calls),len(actions),len(windows))
