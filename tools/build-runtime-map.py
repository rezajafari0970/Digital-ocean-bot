#!/usr/bin/env python3
import os,json,subprocess,hashlib,re,glob
ROOT=os.path.abspath(os.path.join(os.path.dirname(__file__),'..')); os.chdir(ROOT)
def run(a): return subprocess.run(a,text=True,capture_output=True)
def sha(p):
 try:
  h=hashlib.sha256(); h.update(open(p,'rb').read()); return h.hexdigest()
 except:return None
services={}
for s in ['digital-ocean-bot-api','digital-ocean-bot-worker']:
 props={}
 for k in ['ActiveState','SubState','MainPID','FragmentPath','WorkingDirectory','ExecStart','EnvironmentFiles']:
  p=run(['systemctl','show',s,'-p',k,'--value']); props[k]=p.stdout.strip()
 pid=props['MainPID']; exe=''
 try: exe=os.path.realpath('/proc/'+pid+'/exe') if pid and pid!='0' else ''
 except: pass
 props['resolved_executable']=exe; props['binary_sha256']=sha(exe) if exe else None; services[s]=props
idx={'schema_version':1,'canonical_repo':ROOT,'git':{'branch':run(['git','branch','--show-current']).stdout.strip(),'head':run(['git','rev-parse','HEAD']).stdout.strip()},'paths':{'app':'/opt/digital-ocean-bot','bin':'/opt/digital-ocean-bot/bin','env':'/etc/digital-ocean-bot/env','data':'/var/lib/digital-ocean-bot','static':'/opt/digital-ocean-bot/web/static','migrations':'/opt/digital-ocean-bot/migrations'},'services':services,'source_units':['deploy/digital-ocean-bot-api.service','deploy/digital-ocean-bot-worker.service'],'scripts':{'install':'deploy/install.sh','upgrade':'deploy/upgrade.sh','healthcheck':'deploy/healthcheck.sh','bootstrap':'deploy/bootstrap.sh'},'health':{'healthz':'http://127.0.0.1:18080/healthz','readyz':'http://127.0.0.1:18080/readyz'},'build':{'api':'go build -trimpath -ldflags=-s\\ -w -o /opt/digital-ocean-bot/bin/digital-ocean-bot-api ./cmd/api','worker':'go build -trimpath -ldflags=-s\\ -w -o /opt/digital-ocean-bot/bin/digital-ocean-bot-worker ./cmd/worker'},'production_revision_note':'Current binaries are not stamped with Git commit by the existing build scripts; binary hash + deploy time/runtime checks can prove artifact identity but not map it to a commit without a recorded build manifest.'}
json.dump(idx,open('docs/RUNTIME_MAP.json','w',encoding='utf-8'),ensure_ascii=False,indent=2); open('docs/RUNTIME_MAP.json','a').write('\n')
out=['# Deployment / Runtime Map','',f'Canonical source: `{ROOT}` on branch `{idx["git"]["branch"]}` at `{idx["git"]["head"]}`.','', '## Production layout','- App: `/opt/digital-ocean-bot`','- Binaries: `/opt/digital-ocean-bot/bin`','- Environment file: `/etc/digital-ocean-bot/env` (never copy secrets into docs)','- Mutable data: `/var/lib/digital-ocean-bot`','- Static UI: `/opt/digital-ocean-bot/web/static`','- Migrations copy: `/opt/digital-ocean-bot/migrations`','', '## Services']
for s,p in services.items(): out += [f'### `{s}`',f'- State: `{p["ActiveState"]}/{p["SubState"]}`; PID `{p["MainPID"]}`',f'- Unit: `{p["FragmentPath"]}`',f'- Working directory: `{p["WorkingDirectory"]}`',f'- Executable: `{p["resolved_executable"]}`',f'- Binary SHA256: `{p["binary_sha256"] or "unavailable"}`','']
out += ['## Build / deploy path','- `deploy/install.sh` builds API/worker directly from source into `/opt/digital-ocean-bot/bin`, copies migrations/static assets, installs systemd units and enables services.','- `deploy/upgrade.sh` stops services, copies migrations/static assets, builds `.new` binaries, atomically replaces production binaries, then starts services.','- `deploy/healthcheck.sh` checks `/healthz`, `/readyz`, API active and worker active.','', '## Revision identity caveat','The existing build scripts do **not** stamp the Git commit into the binary. Therefore “service active” does not prove production is running the current repository HEAD. Before claiming a deploy is live, use the actual deployment procedure plus health/feature smoke verification. A future hardening step can add an artifact build manifest or version endpoint mapping binary SHA/commit/deploy time.','', 'Full machine-readable runtime snapshot: `docs/RUNTIME_MAP.json`.']
open('docs/RUNTIME_NAVIGATION.md','w',encoding='utf-8').write('\n'.join(out)+'\n')
print('runtime map',[(s,p['ActiveState'],p['MainPID']) for s,p in services.items()])
