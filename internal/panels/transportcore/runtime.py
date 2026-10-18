"""Verified, journaled core/panel installation. Input is injected by Go."""
import contextlib,fcntl,gzip,hashlib,json,os,pathlib,shutil,sqlite3,subprocess,tempfile,time,urllib.parse,urllib.request

def digest(path):
 h=hashlib.sha256()
 with open(path,'rb') as f:
  for chunk in iter(lambda:f.read(1024*1024),b''):h.update(chunk)
 return h.hexdigest()
def run(args,timeout=30,env=None):
 p=subprocess.run(args,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,timeout=timeout,env=env)
 if p.returncode:raise RuntimeError('command_failed')
 return p.stdout
def save(path,data):
 tmp=path.with_suffix('.tmp');tmp.write_text(json.dumps(data,sort_keys=True));tmp.chmod(0o600)
 with open(tmp,'rb') as f:os.fsync(f.fileno())
 os.replace(tmp,path)
 fd=os.open(path.parent,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
def install_file(source,dest):
 temp=dest.with_name(dest.name+'.dob-new');shutil.copyfile(source,temp);temp.chmod(0o755)
 with open(temp,'rb') as f:os.fsync(f.fileno())
 os.replace(temp,dest)
 fd=os.open(dest.parent,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
def identity(db):
 with sqlite3.connect('file:'+str(db)+'?mode=ro',uri=True,timeout=10) as c:
  result={}
  for table,columns in [('users',['id','username','password']),('clients',['id','uuid','email']),('inbounds',['id','port','protocol','settings','stream_settings'])]:
   known={x[1] for x in c.execute('PRAGMA table_info('+table+')')};cols=[x for x in columns if x in known]
   if cols:result[table]=c.execute('SELECT '+','.join(cols)+' FROM '+table+' ORDER BY id').fetchall()
  settings=dict(c.execute("SELECT key,value FROM settings WHERE key IN ('webPort','webBasePath','webCertFile')"))
  if settings.get('webCertFile'):raise RuntimeError('unsupported_https_panel')
  port=int(settings.get('webPort','2053'));path=settings.get('webBasePath','/')
  if not 1<=port<=65535 or not path.startswith('/') or any(x in path for x in ['\r','\n','?','#']):raise RuntimeError('invalid_panel_endpoint')
  return hashlib.sha256(json.dumps(result,sort_keys=True).encode()).hexdigest(),'http://127.0.0.1:'+str(port)+'/'+path.strip('/')+'/csrf-token'
def ready(panel,core,want_panel,want_core,url):
 try:
  if digest(panel)!=want_panel or digest(core)!=want_core:return False
  run(['systemctl','is-active','--quiet','x-ui'],5)
  pid=int(run(['systemctl','show','x-ui','-p','MainPID','--value'],5))
  if digest('/proc/'+str(pid)+'/exe')!=want_panel:return False
  children=set()
  for task in pathlib.Path('/proc/'+str(pid)+'/task').glob('*/children'):children.update(task.read_text().split())
  if not any(digest('/proc/'+child+'/exe')==want_core for child in children if pathlib.Path('/proc/'+child+'/exe').exists()):return False
  with urllib.request.urlopen(url.replace('//csrf-token','/csrf-token'),timeout=2) as r:return r.status==200
 except (OSError,ValueError,RuntimeError,subprocess.SubprocessError):return False
def wait_ready(*args):
 end=time.monotonic()+35
 while time.monotonic()<end:
  if ready(*args):return True
  time.sleep(1)
 return False
def stage(asset,cache):
 dest=cache/asset['name']
 if dest.exists() and digest(dest)==asset['sha256']:return dest
 parsed=urllib.parse.urlsplit(asset['url'])
 if parsed.scheme!='https' or parsed.hostname!='raw.githubusercontent.com':raise RuntimeError('invalid_artifact_origin')
 archive=cache/(asset['name']+'.gz.part')
 with urllib.request.urlopen(asset['url'],timeout=30) as r,open(archive,'wb') as f:
  size=0
  while True:
   b=r.read(1024*1024)
   if not b:break
   size+=len(b)
   if size>64*1024*1024:raise RuntimeError('artifact_too_large')
   f.write(b)
 if digest(archive)!=asset['gzip_sha256']:raise RuntimeError('artifact_digest_mismatch')
 temp=dest.with_suffix('.part')
 with gzip.open(archive,'rb') as r,open(temp,'wb') as f:
  size=0
  while True:
   b=r.read(1024*1024)
   if not b:break
   size+=len(b)
   if size>128*1024*1024:raise RuntimeError('binary_too_large')
   f.write(b)
 if digest(temp)!=asset['sha256']:raise RuntimeError('binary_digest_mismatch')
 temp.chmod(0o700);os.replace(temp,dest);archive.unlink();return dest

def main(spec):
 os.umask(0o077)
 panel=pathlib.Path('/usr/local/x-ui/x-ui');machine=os.uname().machine
 core=panel.parent/('bin/xray-linux-arm64' if machine in ('aarch64','arm64') else 'bin/xray-linux-amd64');db=pathlib.Path('/etc/x-ui/x-ui.db')
 candidate=pathlib.Path(spec['candidate'])
 if not str(candidate).startswith('/root/.dob-transport-check-') or candidate.is_symlink():raise RuntimeError('invalid_candidate_path')
 env=dict(os.environ,XRAY_LOCATION_ASSET=str(core.parent))
 with open(candidate) as f:config=json.load(f)
 needs_panel=any(i.get('streamSettings',{}).get('network')=='xhttp' and i.get('streamSettings',{}).get('finalmask',{}).get('tcp') for i in config.get('inbounds',[]))
 needs_install=spec.get('force_core') is True or needs_panel or any(i.get('streamSettings',{}).get('security')=='reality' for i in config.get('inbounds',[]))
 if not needs_install:
  run([str(core),'run','-test','-config',str(candidate)],30,env);print('{"validated":true,"installed":false}');return
 if machine not in ('x86_64','aarch64','arm64') or (needs_panel and machine!='x86_64'):raise RuntimeError('unsupported_runtime_architecture')
 manifest=spec['manifest'];version=manifest['version']
 if version!='dob-fm-0.2.3':raise RuntimeError('unknown_runtime_version')
 core_asset=manifest['core'] if machine=='x86_64' else manifest['core_arm64']
 cache=pathlib.Path('/var/lib/dob-transport-runtime')/(version+('-fragment' if needs_panel else '-core'));cache.mkdir(parents=True,exist_ok=True,mode=0o700);cache.chmod(0o700)
 lock=open('/run/lock/dob-transport-runtime.lock','w');fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
 state_path=cache/'state.json';state=json.loads(state_path.read_text()) if state_path.exists() else None
 want_core=core_asset['sha256'];want_panel=manifest['panel']['sha256'] if needs_panel else digest(panel)
 current_identity,url=identity(db)
 def restore(previous):
  for key in ('panel','core'):
   if digest(cache/('backup-'+key))!=previous['old_'+key]:raise RuntimeError('backup_digest_mismatch')
  for path,key,new in [(panel,'panel',want_panel),(core,'core',want_core)]:
   if digest(path) not in (previous['old_'+key],new):raise RuntimeError('external_runtime_change')
  run(['systemctl','stop','x-ui'],45)
  for path,key in [(panel,'panel'),(core,'core')]:install_file(cache/('backup-'+key),path)
  run(['systemctl','start','x-ui'],45)
  if not wait_ready(panel,core,previous['old_panel'],previous['old_core'],url):raise RuntimeError('rollback_not_ready')
  previous['phase']='rolled_back';save(state_path,previous)
 if state and state['phase'] not in ('applied','rolled_back'):
  if ready(panel,core,want_panel,want_core,url) and current_identity==state['identity']:
   state['phase']='applied';save(state_path,state)
  else:restore(state);raise RuntimeError('recovered_previous_installation')
 if state and state['phase']=='rolled_back':raise RuntimeError('runtime_installation_requires_review')
 if digest(core)==want_core and digest(panel)==want_panel:
  run([str(core),'run','-test','-config',str(candidate)],30,env)
  if not ready(panel,core,want_panel,want_core,url):raise RuntimeError('installed_runtime_not_ready')
  print('{"validated":true,"installed":true,"changed":false}');return
 if state and state['phase']=='applied':raise RuntimeError('runtime_changed_after_installation')
 if run([str(panel),'-v'],5).decode().strip() not in ('3.8.5','3.8.5-dob-fm.1'):raise RuntimeError('unsupported_native_panel_version')
 run(['systemctl','is-active','--quiet','x-ui'],5)
 staged_core=stage(core_asset,cache);staged_panel=stage(manifest['panel'],cache) if needs_panel else panel
 caps=json.loads(run([str(staged_core),'dob-capabilities'],5))
 if caps.get('schema')!=1 or caps.get('websocket_graceful_close') is not True or caps.get('tls_certificate_snapshot') is not True or caps.get('reality_fallback_bounded') is not True or caps.get('reality_handshake_deadline') is not True or caps.get('reality_fallback_max_connections')!=512 or (needs_panel and caps.get('reality_finalmask_fragment') is not True):raise RuntimeError('core_capability_missing')
 if needs_panel and run([str(staged_panel),'-v'],5).decode().strip()!='3.8.5-dob-fm.1':raise RuntimeError('panel_capability_missing')
 run([str(staged_core),'run','-test','-config',str(candidate)],30,env)
 current=core.parent/'config.json'
 if current.exists():run([str(staged_core),'run','-test','-config',str(current)],30,env)
 previous={'phase':'prepared','identity':current_identity,'old_core':digest(core),'old_panel':digest(panel),'version':version}
 shutil.copyfile(core,cache/'backup-core');shutil.copyfile(panel,cache/'backup-panel')
 with sqlite3.connect('file:'+str(db)+'?mode=ro',uri=True,timeout=10) as source,sqlite3.connect(cache/'backup.db') as target:source.backup(target)
 for name in ('backup-core','backup-panel','backup.db'):
  with open(cache/name,'rb') as f:os.fsync(f.fileno())
 save(state_path,previous)
 try:
  run(['systemctl','stop','x-ui'],45)
  previous['phase']='installing';save(state_path,previous)
  install_file(staged_core,core);install_file(staged_panel,panel)
  run(['systemctl','start','x-ui'],45)
  if not wait_ready(panel,core,want_panel,want_core,url):raise RuntimeError('new_runtime_not_ready')
  after,_=identity(db)
  if after!=current_identity:raise RuntimeError('identity_or_inbound_changed')
  previous['phase']='applied';save(state_path,previous)
 except Exception:
  restore(previous);raise RuntimeError('installation_rolled_back')
 print('{"validated":true,"installed":true,"changed":true}')

try:
 main(SPEC)
except Exception as exc:
 # Config-test errors can contain credentials; never forward core stderr.
 print(json.dumps({'error':'transport_runtime_preflight_failed','stage':str(exc) if isinstance(exc,RuntimeError) else type(exc).__name__}))
 raise SystemExit(1)
