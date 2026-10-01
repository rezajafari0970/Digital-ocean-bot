#!/usr/bin/env python3
import json,hashlib,subprocess,urllib.request,sys,os

def sh(*a): return subprocess.check_output(a,text=True).strip()
def sha(p):
 h=hashlib.sha256()
 with open(p,'rb') as f:
  for b in iter(lambda:f.read(1024*1024),b''): h.update(b)
 return h.hexdigest()
head=sh('git','rev-parse','HEAD')
branch=sh('git','branch','--show-current')
with urllib.request.urlopen('http://127.0.0.1:18080/version',timeout=5) as r: version=json.load(r)
with open('/opt/digital-ocean-bot/build-manifest.json') as f: manifest=json.load(f)
actual={'api_sha256':sha('/opt/digital-ocean-bot/bin/digital-ocean-bot-api'),'worker_sha256':sha('/opt/digital-ocean-bot/bin/digital-ocean-bot-worker')}
services={s:subprocess.run(['systemctl','is-active',s],text=True,capture_output=True).stdout.strip() for s in ['digital-ocean-bot-api','digital-ocean-bot-worker']}
checks={
 'runtime_commit_matches_manifest':version.get('commit')==manifest.get('commit'),
 'manifest_commit_matches_git_head':manifest.get('commit')==head,
 'api_hash_matches_manifest':actual['api_sha256']==manifest.get('api_sha256'),
 'worker_hash_matches_manifest':actual['worker_sha256']==manifest.get('worker_sha256'),
 'api_active':services['digital-ocean-bot-api']=='active',
 'worker_active':services['digital-ocean-bot-worker']=='active',
}
out={'branch':branch,'git_head':head,'runtime_version':version,'artifact_manifest':manifest,'actual_binary_hashes':actual,'services':services,'checks':checks,'verified':all(checks.values())}
os.makedirs('docs',exist_ok=True)
with open('docs/PRODUCTION_REVISION_STATUS.json','w') as f: json.dump(out,f,indent=2); f.write('\n')
print(json.dumps(out,indent=2))
sys.exit(0 if out['verified'] else 1)
