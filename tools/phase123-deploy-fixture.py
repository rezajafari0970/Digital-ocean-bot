#!/usr/bin/env python3
"""Execute complete deployment scripts against a temporary host model only.
Absolute application/config/data/unit paths are redirected in COPIES. All
process/service/build/credential commands are mocked; no production mutation.
"""
from pathlib import Path
import os,json,base64,shutil,subprocess,tempfile,sys,hashlib,fcntl
ROOT=Path(__file__).resolve().parents[1]
RESULT=[]
MOCK=r'''
import os,sys,json,pathlib
name=pathlib.Path(sys.argv[0]).name
root=pathlib.Path(os.environ['FIXTURE_ROOT'])
args=sys.argv[1:]
with (root/'commands.jsonl').open('a') as f:f.write(json.dumps({'command':name,'args':args})+'\n')
if name=='systemctl':
 state=json.loads((root/'state.json').read_text())
 unit=lambda x:x.removesuffix('.service')
 if args[0] in ('is-active','is-enabled'):
  prop='active' if args[0]=='is-active' else 'enabled'
  sys.exit(0 if unit(args[-1]) in state[prop] else 3)
 if args[0]=='cat':sys.exit(0 if (root/'units'/(unit(args[-1])+'.service')).exists() else 1)
 if args[0]=='show':
  u=unit(args[1]);prop=args[args.index('-p')+1]
  if prop=='LoadState':print('loaded' if (root/'units'/(u+'.service')).exists() else 'not-found')
  elif prop=='ActiveState':print('active' if u in state['active'] else 'inactive')
  elif prop=='MainPID':print('123' if u in state['active'] else '0')
  elif prop=='Type':print('notify')
  elif prop=='WatchdogUSec':print('45s')
  elif prop=='ExecStart':
   argv=str(root/'app/bin'/('digital-ocean-bot-api' if u.endswith('-api') else 'digital-ocean-bot-worker'))
   if not u.endswith('-api'):argv+=' --role='+('panels' if u.endswith('-panels') else 'control')
   print('{ path=fixture ; argv[]='+argv+' ; ignore_errors=no ; }')
  sys.exit(0)
 if args[0]=='disable' and any(not (root/'units'/(unit(u)+'.service')).exists() for u in args[1:]):sys.exit(5)
 if args[0]=='stop' and any(not (root/'units'/(unit(u)+'.service')).exists() for u in args[1:]):sys.exit(5)
 if args[0]==os.environ.get('FAIL_ROLLBACK') and args[-1]=='digital-ocean-bot-api' and (root/'failed-once').exists():sys.exit(1)
 if args[0] in ('start','restart') and os.environ.get('FAIL_ACTIVATION')=='1' and not (root/'failed-once').exists():
  (root/'failed-once').touch();sys.exit(1)
 if args[0]=='enable' and os.environ.get('FALSE_ENABLE')=='1' and len(args)>2:sys.exit(0)
 if args[0] in ('start','restart','stop','enable','disable'):
  prop='enabled' if args[0] in ('enable','disable') else 'active'
  for raw in args[1:]:
   u=unit(raw)
   if args[0] in ('start','restart','enable'):
    if u not in state[prop]:state[prop].append(u)
   else:
    state[prop]=[v for v in state[prop] if v!=u]
  if args[0] in ('start','restart') and len(args)>2 and os.environ.get('BROWSER_EXIT')=='1':state['active']=[v for v in state['active'] if v!='digital-ocean-bot-vultr-browser-manager']
  (root/'state.json').write_text(json.dumps(state))
 sys.exit(0)
if name=='mv':
 if os.environ.get('FAIL_PUBLISH')=='1' and args[-1].endswith('/migrations') and not (root/'publish-failed').exists():
  (root/'publish-failed').touch();sys.exit(1)
 os.execv('/usr/bin/mv',['mv',*args])
if name=='go':
 if os.environ.get('FAIL_BUILD')=='1' and args[-1]=='./cmd/worker':sys.exit(1)
 out=pathlib.Path(args[args.index('-o')+1]);out.parent.mkdir(parents=True,exist_ok=True);out.write_text('new fixture binary');sys.exit(0)
if name=='python3':
 if args and args[0].endswith('/bootstrap.py'):os.execv(sys.executable,[sys.executable,*args])
 body=sys.stdin.read()
 if 'DOB_CHECKPOINT_ROLLBACK_GUARD' in body:
  sys.exit(1 if os.environ.get('PENDING_CHECKPOINT')=='1' else 0)
 if 'split topology readiness' in body:sys.exit(0)
 raise SystemExit('unexpected fixture python invocation')
if name=='openssl':print('a'*48 if '-hex' in args else 'fixture-master-key');sys.exit(0)
if name=='sudo':
 sys.stdin.read();(root/'database-bootstrapped').touch();sys.exit(0)
if name=='psql':print('1');sys.exit(0)
if name=='id':print('0');sys.exit(0)
if name in ('chown','chmod','useradd','runuser'):sys.exit(0)
if name=='install':
 cleaned=[];i=0
 while i<len(args):
  if args[i] in ('-o','-g'):i+=2
  else:cleaned.append(args[i]);i+=1
 os.execv('/usr/bin/install',['install',*cleaned])
raise SystemExit('unhandled mock '+name)
'''
def run_case(mode,failure=False,pending=False,conflict=False,build_failure=False,publish_failure=False,overlap=False,rollback_failure='', false_enable=False, browser_exit=False):
 with tempfile.TemporaryDirectory(prefix='dob-deploy-audit-') as temp:
  root=Path(temp);src=root/'source';src.mkdir()
  shutil.copytree(ROOT/'deploy',src/'deploy')
  (src/'migrations').mkdir();(src/'migrations/fixture.sql').write_text('SELECT 1;')
  (src/'web/static').mkdir(parents=True);(src/'web/static/index.html').write_text('fixture')
  for name in ['app/bin','etc','data','units','mockbin']:(root/name).mkdir(parents=True,exist_ok=True)
  if mode!='fresh':
   (root/'etc/env').write_text('DATABASE_URL=postgres://user:password@127.0.0.1:5432/fixture?sslmode=disable\nMASTER_KEY_FILE='+str(root/'etc/master.key')+'\nMASTER_KEY_VERSION=1\nHTTP_ADDR=127.0.0.1:18080\n')
   (root/'etc/master.key').write_bytes(base64.b64encode(bytes(32))+b'\n')
  services=['digital-ocean-bot-vultr-browser-manager','digital-ocean-bot-api','digital-ocean-bot-worker']
  initial={'active':[],'enabled':[]}
  if mode!='fresh':
   initial={'active':services.copy(),'enabled':services.copy()}
   for u in services:(root/'units'/(u+'.service')).write_text('old unit '+u)
   for name in ['digital-ocean-bot-api','digital-ocean-bot-worker','vultr-browser-session','vultr-browser-manager','vultr-input-bridge']:(root/'app/bin'/name).write_text('old fixture binary')
  if conflict:
   drop=root/'units/digital-ocean-bot-worker.service.d';drop.mkdir()
   directive={'exec':'ExecStart=/old/worker --role=all','unset':'UnsetEnvironment=DOB_WORKER_MODE','envfile':'EnvironmentFile=/operator/env'}[conflict]
   (drop/'99-operator.conf').write_text('[Service]\n'+directive+'\n')
  def artifacts():
   return {str(p.relative_to(root)):hashlib.sha256(p.read_bytes()).hexdigest() for area in ['app','units','etc','data'] for p in (root/area).rglob('*') if p.is_file()}
  if mode!='fresh':
   (root/'app/migrations').mkdir()
   (root/'app/migrations/prior.sql').write_text('prior migration')
   (root/'app/web/static').mkdir(parents=True)
   (root/'app/web/static/index.html').write_text('prior static')
   (root/'app/build-manifest.json').write_text('{"commit":"prior"}')
  before_artifacts=artifacts()
  (root/'state.json').write_text(json.dumps(initial))
  for name in ['systemctl','go','python3','id','chown','chmod','useradd','runuser','install','mv','openssl','sudo','psql']:
   path=root/'mockbin'/name;path.write_text('#!'+sys.executable+'\n'+MOCK);path.chmod(0o755)
  for name in ['install.sh','upgrade.sh','service-topology.sh']:
   p=src/'deploy'/name;s=p.read_text()
   for original,replacement in [('/opt/digital-ocean-bot',str(root/'app')),('/etc/digital-ocean-bot',str(root/'etc')),('/var/lib/digital-ocean-bot',str(root/'data')),('/etc/systemd/system',str(root/'units'))]:s=s.replace(original,replacement)
   s=s.replace('export PATH=/usr/local/go/bin:/usr/sbin:/usr/bin:/sbin:/bin','export PATH="'+str(root/'mockbin')+':/usr/local/go/bin:/usr/sbin:/usr/bin:/sbin:/bin"')
   p.write_text(s)
  env=os.environ.copy();env.update({'SRC':str(src),'FIXTURE_ROOT':str(root),'PATH':str(root/'mockbin')+':/usr/bin:/bin','FAIL_ACTIVATION':'1' if failure else '0','PENDING_CHECKPOINT':'1' if pending else '0','DOB_UNIT_DIR':str(root/'units'),'FAIL_BUILD':'1' if build_failure else '0','FAIL_PUBLISH':'1' if publish_failure else '0','FAIL_ROLLBACK':rollback_failure,'FALSE_ENABLE':'1' if false_enable else '0','BROWSER_EXIT':'1' if browser_exit else '0'})
  name='install.sh' if mode=='fresh' else 'upgrade.sh'
  lock=None
  if overlap:
   lock=(root/'.digital-ocean-bot-deploy.lock').open('a+');fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
  proc=subprocess.run(['bash',str(src/'deploy'/name)],env=env,capture_output=True,text=True,timeout=30)
  if lock is not None:lock.close()
  state=json.loads((root/'state.json').read_text())
  commands=[json.loads(l) for l in (root/'commands.jsonl').read_text().splitlines()]
  if overlap:
   assert proc.returncode!=0 and state==initial and artifacts()==before_artifacts,(proc.stderr,state)
   assert 'Another deployment' in proc.stderr
   assert not any(x['command'] in ('go','mv','install') or (x['command']=='systemctl' and x['args'][0] in ('stop','start','restart','daemon-reload')) for x in commands)
  elif build_failure:
   assert proc.returncode!=0 and state==initial,(proc.returncode,state,proc.stderr)
   assert artifacts()==before_artifacts,'failed staged build changed runtime artifacts'
   assert not any(x['command']=='systemctl' and x['args'][0] in ('stop','start','restart','daemon-reload') for x in commands)
  elif conflict:
   assert proc.returncode!=0 and state==initial and 'Conflicting unowned' in proc.stderr,(proc.returncode,state,proc.stderr)
   assert (root/'app/bin/digital-ocean-bot-worker').read_text()=='old fixture binary'
   assert artifacts()==before_artifacts,'preflight refusal changed managed runtime artifacts'
   assert not any(x['command']=='systemctl' and x['args'][0] in ('stop','start','restart','daemon-reload') for x in commands)
  elif not failure and not publish_failure and not false_enable and not browser_exit:
   assert proc.returncode==0,proc.stderr
   for asset in ['migrations/fixture.sql','web/static/index.html']:
    assert (root/'app'/asset).read_bytes()==(src/asset).read_bytes(),'stale release asset '+asset
   assert not (root/'app/migrations/prior.sql').exists()
   manifest=json.loads((root/'app/build-manifest.json').read_text())
   for key,binary in [('api_sha256','digital-ocean-bot-api'),('worker_sha256','digital-ocean-bot-worker')]:
    assert manifest[key]==hashlib.sha256((root/'app/bin'/binary).read_bytes()).hexdigest()

   expected=set(services+['digital-ocean-bot-worker-panels'])
   assert set(state['active'])==expected and set(state['enabled'])==expected,state
   assert 'DOB_WORKER_MODE=split' in (root/'units/digital-ocean-bot-api.service.d/60-worker-isolation.conf').read_text()
   assert '--role=control' in (root/'units/digital-ocean-bot-worker.service.d/60-worker-isolation.conf').read_text()
   assert any(x['command']=='systemctl' and x['args']==['daemon-reload'] for x in commands)
  elif rollback_failure:
   assert proc.returncode!=0 and not state['active'],(proc.returncode,state,proc.stderr)
   assert artifacts()==before_artifacts,'failed rollback start damaged restored artifacts'
   assert 'Rollback '+rollback_failure+' failed: digital-ocean-bot-api' in proc.stderr
   assert 'Rollback service restoration failed; managed services remain stopped' in proc.stderr
   assert 'Deployment failed; restored matching' not in proc.stderr
  elif pending:
   assert proc.returncode!=0 and not state['active'],(proc.returncode,state,proc.stderr)
   assert 'Rollback refused' in proc.stderr
   assert (root/'app/bin/digital-ocean-bot-worker').read_text()=='new fixture binary'
  else:
   assert proc.returncode!=0 and state==initial,(proc.returncode,state,initial,proc.stderr)
   if mode=='fresh':
    now=artifacts()
    assert {k:v for k,v in now.items() if k.startswith(('app/','units/'))}=={k:v for k,v in before_artifacts.items() if k.startswith(('app/','units/'))},'fresh release rollback incomplete'
    credentials={k:v for k,v in now.items() if k.startswith('etc/')}
    assert set(credentials)=={'etc/env','etc/master.key'}
    assert (root/'database-bootstrapped').exists() and not (root/'etc/bootstrap.pending').exists()
    retry=subprocess.run(['bash',str(src/'deploy'/name)],env=env,capture_output=True,text=True,timeout=30)
    assert retry.returncode==0,retry.stderr
    assert credentials=={k:v for k,v in artifacts().items() if k.startswith('etc/')},'fresh retry changed DB/master credentials'
   else:
    assert artifacts()==before_artifacts,'rollback did not restore the complete release'
    assert (root/'app/bin/digital-ocean-bot-worker').read_text()=='old fixture binary'
    assert (root/'units/digital-ocean-bot-worker.service').read_text().startswith('old unit ')
    assert not (root/'units/digital-ocean-bot-worker-panels.service').exists()
  RESULT.append({'case':mode,'activation_failure':failure,'build_failure':build_failure,'rollback_failure':rollback_failure,'false_enable':false_enable,'browser_exit':browser_exit,'overlap':overlap,'publish_failure':publish_failure,'pending_checkpoint':pending,'conflicting_override':conflict,'status':'PASS','scope':'full shell execution with redirected paths and mocked OS commands'})
for args in [('fresh',False,False),('legacy-upgrade',False,False),('legacy-upgrade',True,False),('legacy-upgrade',True,True),('legacy-upgrade',False,False,'exec'),('legacy-upgrade',False,False,'unset'),('legacy-upgrade',False,False,'envfile'),('legacy-upgrade',False,False,False,True),('fresh',False,False,False,True),('legacy-upgrade',False,False,False,False,True),('legacy-upgrade',False,False,False,False,False,True),('fresh',True,False)]:run_case(*args)
for failure in ('enable','start'):
 run_case('legacy-upgrade',failure=True,rollback_failure=failure)
run_case('legacy-upgrade',false_enable=True)
run_case('legacy-upgrade',browser_exit=True)
print(json.dumps({'status':'PASS','cases':RESULT,'production_services_changed':False},indent=2))
