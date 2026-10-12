#!/usr/bin/env python3
import datetime,json,os,pathlib,runpy,secrets,shutil,subprocess,sys
from urllib.parse import urlsplit,unquote
ROOT=pathlib.Path(__file__).resolve().parent
helpers=runpy.run_path(str(ROOT/'bootstrap.py'))
ETC=pathlib.Path('/etc/digital-ocean-bot')
STATE=ETC/'installation.json'
GID=int(subprocess.check_output(['id','-g','digitaloceanbot'],text=True))
def atomic(path,data,mode=0o600):
 helpers['atomic_write'](path,data.encode(),mode,GID)
def peer(sql):
 p=subprocess.run(['runuser','-u','postgres','--','psql','-XAt','-v','ON_ERROR_STOP=1'],input=sql,text=True,capture_output=True,timeout=45)
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
  collision=peer("SELECT (EXISTS(SELECT FROM pg_roles WHERE rolname='digitaloceanbot') OR EXISTS(SELECT FROM pg_database WHERE datname='digital_ocean_bot'))::int;")
  if collision!='0': raise RuntimeError('Database or role already exists; refusing to adopt/reset it')
  atomic(STATE,json.dumps({'status':'pending','created_at':datetime.datetime.now(datetime.timezone.utc).isoformat()})+'\n')
  fresh=True
 helpers['prepare'](str(ETC),'digital_ocean_bot','digitaloceanbot',GID,validate_only=not fresh)
 if fresh:
  if not (ETC/'setup-token').exists(): atomic(ETC/'setup-token',secrets.token_urlsafe(32)+'\n')
  if (ETC/'bootstrap.pending').exists():
   u=urlsplit(values()['DATABASE_URL']);password=unquote(u.password)
   if u.username!='digitaloceanbot' or u.path!='/digital_ocean_bot' or len(password)!=48 or any(c not in '0123456789abcdef' for c in password):
    raise RuntimeError('Fresh bootstrap identity mismatch')
   if peer("SELECT count(*) FROM pg_roles WHERE rolname='digitaloceanbot';")=='0':
    peer("CREATE ROLE digitaloceanbot LOGIN PASSWORD '"+password+"';")
   else:
    checkenv=dict(os.environ,PGHOST='127.0.0.1',PGPORT='5432',PGDATABASE='postgres',PGUSER='digitaloceanbot',PGPASSWORD=password)
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
 b.mkdir(parents=True,mode=0o700);os.chmod(b.parent,0o700)
 shutil.copytree(ETC,b/'configuration')
 with (b/'database.dump').open('wb') as out:
  p=subprocess.run(['pg_dump','--format=custom','--no-owner'],env=db_env(),stdout=out,stderr=subprocess.PIPE,timeout=600)
 if p.returncode:raise RuntimeError('Database backup failed; live services unchanged')
 shutil.copy2('/opt/digital-ocean-bot/build-manifest.json',b/'build-manifest.json')
 print('Private database/configuration backup: '+str(b))
def complete():
 if STATE.exists():
  state=json.loads(STATE.read_text());state['status']='installed';atomic(STATE,json.dumps(state)+'\n')
def status():
 print(query("SELECT json_build_object('port',listen_port,'path',web_path,'host',listen_host) FROM panel_settings WHERE id=true;"))
def main():
 mode=sys.argv[1]
 if mode=='prepare':prepare()
 elif mode=='bootstrap':bootstrap(sys.argv[2])
 elif mode=='backup':backup()
 elif mode=='complete':complete()
 elif mode=='address':status()
 else:raise RuntimeError('Unknown installer operation')
if __name__=='__main__':
 try:main()
 except Exception as exc:
  print('Installer state error: '+(str(exc) if isinstance(exc,RuntimeError) else type(exc).__name__),file=sys.stderr);sys.exit(1)
