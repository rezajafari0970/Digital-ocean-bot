#!/usr/bin/env python3
"""Run inside the isolated publisher test container, never production."""
import argparse,hashlib,json,os,pathlib,secrets,subprocess,urllib.request,urllib.error
BASE='http://127.0.0.1:18080'
def request(path,payload=None,token='',grant=''):
 headers={'Content-Type':'application/json','Authorization':'Bearer '+token,'X-Bootstrap-Token':grant}
 req=urllib.request.Request(BASE+path,data=json.dumps(payload).encode() if payload is not None else None,headers=headers)
 try:
  with urllib.request.urlopen(req,timeout=20) as res:return res.status,json.loads(res.read() or b'null')
 except urllib.error.HTTPError as e:return e.code,json.loads(e.read() or b'null')
def fingerprints():
 import runpy
 source=pathlib.Path('/usr/local/lib/datacenter-bot/source').read_text().strip()
 funcs=runpy.run_path(source+'/deploy/bootstrap.py')
 env=funcs['database_environment'](funcs['read_environment'](pathlib.Path('/etc/digital-ocean-bot/env')))
 p=subprocess.run(['psql','-XAt','-c',"SELECT id::text||':'||username||':'||password_hash FROM admin_users ORDER BY id"],env=env,capture_output=True,check=True)
 return {'key':hashlib.sha256(pathlib.Path('/etc/digital-ocean-bot/master.key').read_bytes()).hexdigest(),'env':hashlib.sha256(pathlib.Path('/etc/digital-ocean-bot/env').read_bytes()).hexdigest(),'users':hashlib.sha256(p.stdout).hexdigest()}
def main():
 global BASE
 parser=argparse.ArgumentParser();parser.add_argument('--before',action='store_true');parser.add_argument('--revision');a=parser.parse_args()
 snapshot=pathlib.Path('/root/release-before.json');password_file=pathlib.Path('/root/release-test-password')
 if a.before:
  if pathlib.Path('/usr/local/lib/datacenter-bot/source').exists():snapshot.write_text(json.dumps(fingerprints()))
  return
 source=pathlib.Path('/usr/local/lib/datacenter-bot/source').read_text().strip()
 import runpy
 helpers=runpy.run_path(source+'/deploy/bootstrap.py')
 env=helpers['database_environment'](helpers['read_environment'](pathlib.Path('/etc/digital-ocean-bot/env')))
 port=subprocess.check_output(['psql','-XAt','-c','SELECT listen_port FROM panel_settings WHERE id=true'],env=env,text=True).strip()
 BASE='http://127.0.0.1:'+port
 status,ready=request('/readyz');assert status==200 and len(ready['checks'])==5 and all(x['ok']for x in ready['checks'])
 version=json.loads(pathlib.Path('/opt/digital-ocean-bot/build-manifest.json').read_text());assert version['commit']==a.revision
 assert request('/version')[1]['commit']==a.revision
 subprocess.run(['python3',source+'/deploy/verify-panel.py','--bootstrap',source+'/deploy/bootstrap.py','--etc','/etc/digital-ocean-bot','--static-root',source+'/web/static'],check=True)
 for unit in ['digital-ocean-bot-api','digital-ocean-bot-worker','digital-ocean-bot-worker-panels']:
  subprocess.run(['systemctl','is-active','--quiet',unit],check=True);subprocess.run(['systemctl','is-enabled','--quiet',unit],check=True)
 assert subprocess.run(['systemctl','is-active','--quiet','digital-ocean-bot-vultr-browser-manager']).returncode!=0
 if snapshot.exists():assert json.loads(snapshot.read_text())==fingerprints(),'Update changed credentials/identity'
 if not password_file.exists():
  status,result=request('/api/v1/auth/login',{'username':'admin','password':'admin'})
  assert status==200 and not result['user']['MustChangePassword']
  assert request('/api/v1/accounts',token=result['token'])[0]==200
  assert request('/api/v1/auth/logout',{},result['token'])[0]==204
  password_file.write_text('admin');password_file.chmod(0o600)
 password=password_file.read_text()
 status,result=request('/api/v1/auth/login',{'username':'admin','password':password})
 assert status==200 and not result['user']['MustChangePassword']
 assert request('/api/v1/accounts',token=result['token'])[0]==200
 settings=request('/api/v1/panel-settings',token=result['token'])[1]
 assert 'listen_host' not in settings and settings['username']=='admin'
 status,update=request('/api/v1/project-update',token=result['token'])
 assert status==200 and update['available']
 assert request('/api/v1/auth/logout',{},result['token'])[0]==204
 if snapshot.exists():snapshot.unlink()
 print(json.dumps({'smoke':'PASS','revision':a.revision,'ready_checks':5,'identity_preserved':True,'ordinary_login_verified':True}))
if __name__=='__main__':main()
