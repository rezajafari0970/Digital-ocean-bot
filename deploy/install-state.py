#!/usr/bin/env python3
import datetime,json,os,pathlib,runpy,secrets,shutil,subprocess,sys,tarfile,tempfile
from backup_store import seal,prune
from owned_cluster import plan_cluster,ensure_cluster
from urllib.parse import urlsplit,unquote
ROOT=pathlib.Path(__file__).resolve().parent
helpers=runpy.run_path(str(ROOT/'bootstrap.py'))
ETC=pathlib.Path('/etc/digital-ocean-bot')
STATE=ETC/'installation.json'
GID=int(subprocess.check_output(['id','-g','digitaloceanbot'],text=True))
def atomic(path,data,mode=0o600):
 helpers['atomic_write'](path,data.encode(),mode,GID)
def peer(sql):
 port=str(urlsplit(values()['DATABASE_URL']).port or 5432) if (ETC/'env').exists() else '5432'
 p=subprocess.run(['runuser','-u','postgres','--','psql','-h','/var/run/postgresql','-p',port,'-d','postgres','-XAt','-v','ON_ERROR_STOP=1'],input=sql,text=True,capture_output=True,timeout=45)
 if p.returncode: raise RuntimeError('PostgreSQL bootstrap failed; credentials were preserved')
 return p.stdout.strip()
def values(): return helpers['read_environment'](ETC/'env')
def db_env():
 return helpers['database_environment'](values())
def query(sql):
 p=subprocess.run(['psql','-XAt','-v','ON_ERROR_STOP=1'],env=db_env(),input=sql,text=True,capture_output=True,timeout=30)
 if p.returncode: raise RuntimeError('Database check failed; credentials were preserved')
 return p.stdout.strip()
def prepare():
 fresh=STATE.exists() and json.loads(STATE.read_text()).get('status')=='pending'
 if not STATE.exists() and not (ETC/'env').exists():
  if (ETC/'master.key').exists() or pathlib.Path('/opt/digital-ocean-bot/bin').exists():
   raise RuntimeError('Ambiguous existing installation; refusing fresh defaults')
  atomic(STATE,json.dumps({'status':'pending','created_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'cluster':plan_cluster()})+'\n')
  fresh=True
 state=json.loads(STATE.read_text()) if STATE.exists() else {}
 port=5432
 if state.get('cluster'):
  port=ensure_cluster(state,lambda value: atomic(STATE,json.dumps(value)+'\n'))
 if (ETC/'env').exists() and state.get('cluster'):
  u=urlsplit(values()['DATABASE_URL'])
  if u.hostname!='127.0.0.1' or u.port!=port:raise RuntimeError('Persisted cluster endpoint mismatch')
 helpers['prepare'](str(ETC),'digital_ocean_bot','digitaloceanbot',GID,validate_only=not fresh,port=port)
 if fresh:
  if (ETC/'bootstrap.pending').exists():
   u=urlsplit(values()['DATABASE_URL']);password=unquote(u.password)
   if u.username!='digitaloceanbot' or u.path!='/digital_ocean_bot' or len(password)!=48 or any(c not in '0123456789abcdef' for c in password):
    raise RuntimeError('Fresh bootstrap identity mismatch')
   if peer("SELECT count(*) FROM pg_roles WHERE rolname='digitaloceanbot';")=='0':
    peer("CREATE ROLE digitaloceanbot LOGIN PASSWORD '"+password+"';")
   else:
    checkenv=dict(os.environ,PGHOST='127.0.0.1',PGPORT=str(u.port or 5432),PGDATABASE='postgres',PGUSER='digitaloceanbot',PGPASSWORD=password)
    p=subprocess.run(['psql','-XAt','-c','SELECT 1'],env=checkenv,capture_output=True,timeout=15)
    if p.returncode:raise RuntimeError('Existing role does not match persisted installation')
   if peer("SELECT count(*) FROM pg_database WHERE datname='digital_ocean_bot';")=='0':
    peer("CREATE DATABASE digital_ocean_bot OWNER digitaloceanbot;")
   if peer("SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname='digital_ocean_bot';")!='digitaloceanbot':
    raise RuntimeError('Database ownership mismatch')
   query('SELECT 1;')
   (ETC/'bootstrap.pending').unlink()
 print('Database and encryption identity ready; fresh setup pending' if fresh else 'Existing identity preserved')
def bootstrap(app):
 env=dict(os.environ,**values())
 env['DOB_FRESH_INSTALL']='1' if STATE.exists() and json.loads(STATE.read_text()).get('status')=='pending' else '0'
 subprocess.run([str(pathlib.Path(app)/'bin/install-bootstrap')],cwd=app,env=env,check=True)
def backup():
 if not pathlib.Path('/opt/digital-ocean-bot/build-manifest.json').exists(): return
 b=pathlib.Path('/var/backups/datacenter-bot')/datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%S.%fZ')
 b.parent.mkdir(parents=True,mode=0o700,exist_ok=True);os.chmod(b.parent,0o700)
 target=b
 b=pathlib.Path(tempfile.mkdtemp(prefix='.pending-',dir=target.parent))
 shutil.copytree(ETC,b/'configuration')
 with (b/'database.dump').open('wb') as out:
  p=subprocess.run(['pg_dump','--format=custom','--no-owner'],env=db_env(),stdout=out,stderr=subprocess.PIPE,timeout=600)
 if p.returncode:raise RuntimeError('Database backup failed; release publication blocked')
 # Readers are stopped by the caller; preserve the entire persistent directory.
 data=pathlib.Path('/var/lib/digital-ocean-bot')
 if data.exists():
  with tarfile.open(b/'persistent-data.tar','w',dereference=False) as archive:
   archive.add(data,arcname='digital-ocean-bot')
  with tarfile.open(b/'persistent-data.tar','r') as archive:
   if not archive.getmembers():raise RuntimeError('Persistent data backup is empty')
 subprocess.run(['pg_restore','--file=/dev/null',str(b/'database.dump')],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,check=True,timeout=600)
 shutil.copy2('/opt/digital-ocean-bot/build-manifest.json',b/'build-manifest.json')
 # Flush completed backups before any release publication.
 for path in b.rglob('*'):
  if path.is_file() and not path.is_symlink():
   with path.open('rb') as saved:os.fsync(saved.fileno())
 fd=os.open(b,os.O_DIRECTORY)
 try:os.fsync(fd)
 finally:os.close(fd)
 seal(b,target)
 print('Private database/configuration backup: '+str(target))
def complete():
 if STATE.exists():
  state=json.loads(STATE.read_text());state['status']='installed';atomic(STATE,json.dumps(state)+'\n')
def status():
 print(query("SELECT json_build_object('port',listen_port,'path',web_path,'host','0.0.0.0') FROM panel_settings WHERE id=true;"))
def main():
 mode=sys.argv[1]
 if mode=='prepare':prepare()
 elif mode=='bootstrap':bootstrap(sys.argv[2])
 elif mode=='backup':backup()
 elif mode=='complete':complete()
 elif mode=='prune-backups':
  root=pathlib.Path('/var/backups/datacenter-bot')
  if root.exists():print('Expired verified backups removed: '+str(len(prune(root,keep=3))))
 elif mode=='address':status()
 else:raise RuntimeError('Unknown installer operation')
if __name__=='__main__':
 try:main()
 except Exception as exc:
  print('Installer state error: '+(str(exc) if isinstance(exc,RuntimeError) else type(exc).__name__),file=sys.stderr);sys.exit(1)
