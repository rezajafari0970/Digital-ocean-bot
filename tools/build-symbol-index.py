#!/usr/bin/env python3
import os,re,json,glob
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT)
files=sorted(glob.glob('cmd/**/*.go',recursive=True)+glob.glob('internal/**/*.go',recursive=True))
syms=[]
for p in files:
    try: lines=open(p,encoding='utf-8').read().splitlines()
    except: continue
    pkg=''
    for i,line in enumerate(lines,1):
        m=re.match(r'\s*package\s+(\w+)',line)
        if m: pkg=m.group(1)
        m=re.match(r'\s*func\s+(?:\(([^)]*)\)\s*)?([A-Za-z_]\w*)\s*\(',line)
        if m:
            recv=(m.group(1) or '').strip(); syms.append({'kind':'method' if recv else 'func','name':m.group(2),'receiver':recv,'package':pkg,'file':p,'line':i})
        m=re.match(r'\s*type\s+([A-Za-z_]\w*)\s+(struct|interface)\b',line)
        if m: syms.append({'kind':m.group(2),'name':m.group(1),'receiver':'','package':pkg,'file':p,'line':i})
server=open('internal/adminapi/server.go',encoding='utf-8').read().splitlines(); routes=[]
for i,line in enumerate(server,1):
    m=re.search(r'm\.Handle(?:Func)?\("([A-Z]+) ([^" ]+)"\s*,\s*(?:s\.require\()?s\.([A-Za-z0-9_]+)',line)
    if m:
        handler=m.group(3); hits=[s for s in syms if s['name']==handler and s['package']=='adminapi']; routes.append({'method':m.group(1),'path':m.group(2),'handler':handler,'registration_file':'internal/adminapi/server.go','registration_line':i,'handler_definition':hits[0] if hits else None})
idx={'schema_version':1,'symbol_count':len(syms),'route_count':len(routes),'symbols':syms,'routes':routes}
json.dump(idx,open('docs/SYMBOL_INDEX.json','w',encoding='utf-8'),ensure_ascii=False,indent=2); open('docs/SYMBOL_INDEX.json','a').write('\n')
focus=['adminapi','providers','digitalocean','vultr','vultrconsole','sanaei','app','droplets','network','accounts']
by={k:[] for k in focus}
for s in syms:
    if s['package'] in by and (s['kind'] in ('struct','interface') or s['name'][0:1].isupper()): by[s['package']].append(s)
out=['# Symbol Navigation Index','',f'Generated from source: **{len(syms)} symbols**, **{len(routes)} HTTP routes**. Full machine-readable index: `docs/SYMBOL_INDEX.json`.','', '## HTTP route → handler definitions','', '| Method | Path | Handler | Definition |','|---|---|---|---|']
for r in routes:
    h=r['handler_definition']; loc=f"`{h['file']}:{h['line']}`" if h else 'unresolved'; out.append(f"| {r['method']} | `{r['path']}` | `{r['handler']}` | {loc} |")
for pkg in focus:
    vals=by[pkg][:80]
    if not vals: continue
    out += ['',f'## Package `{pkg}` key exported symbols','']
    for s in vals: out.append(f"- `{s['kind']} {s['name']}` — `{s['file']}:{s['line']}`")
open('docs/SYMBOL_NAVIGATION.md','w',encoding='utf-8').write('\n'.join(out)+'\n')
print('symbols',len(syms),'routes',len(routes),'resolved',sum(bool(r['handler_definition']) for r in routes))
