#!/usr/bin/env python3
import json, re, subprocess, glob, os, datetime
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT)
def sh(*a): return subprocess.check_output(a,text=True).strip()
head=sh('git','rev-parse','HEAD'); branch=sh('git','branch','--show-current')
server=open('internal/adminapi/server.go',encoding='utf-8').read()
routes=[]
for m in re.finditer(r'm\.Handle(?:Func)?\("([A-Z]+) ([^" ]+)"\s*,\s*(?:s\.require\()?s\.([A-Za-z0-9_]+)',server): routes.append({'method':m.group(1),'path':m.group(2),'handler':m.group(3)})
path='docs/PROJECT_MANIFEST.json'; old=json.load(open(path,encoding='utf-8')) if os.path.exists(path) else {}
old.update({'schema_version':1,'generated_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'canonical_repo':ROOT,'remote':sh('git','remote','get-url','origin'),'branch':branch,'head_at_generation':head,'routes':routes,'recent_migrations':[os.path.basename(x) for x in sorted(glob.glob('migrations/*.up.sql'))[-12:]],'inventory':{'go_files':len(glob.glob('cmd/**/*.go',recursive=True))+len(glob.glob('internal/**/*.go',recursive=True)),'tests':len(glob.glob('internal/**/*_test.go',recursive=True)),'up_migrations':len(glob.glob('migrations/*.up.sql'))}})
with open(path,'w',encoding='utf-8') as f: json.dump(old,f,ensure_ascii=False,indent=2); f.write('\n')
status={}
for svc in ('digital-ocean-bot-api','digital-ocean-bot-worker'):
    p=subprocess.run(['systemctl','is-active',svc],text=True,capture_output=True); status[svc]=(p.stdout.strip() or 'unknown')
snap={'generated_at_utc':old['generated_at_utc'],'branch':branch,'head':head,'services':status,'inventory':old['inventory'],'route_count':len(routes),'working_tree':subprocess.run(['git','status','--short'],text=True,capture_output=True).stdout.splitlines(),'recent_commits':sh('git','log','-8','--oneline','--decorate').splitlines(),'current_work':old.get('current_work',{})}
with open('docs/CURRENT_SNAPSHOT.json','w',encoding='utf-8') as f: json.dump(snap,f,ensure_ascii=False,indent=2); f.write('\n')
print(f"updated manifest+snapshot head={head[:12]} routes={len(routes)} go={old['inventory']['go_files']} tests={old['inventory']['tests']} migrations={old['inventory']['up_migrations']}")
