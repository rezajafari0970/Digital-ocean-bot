from pathlib import Path
import os,json,subprocess,time,urllib.parse,uuid,base64,datetime
r=Path(__file__).resolve().parents[1]
b=Path(os.environ['DOB_ISOLATION_EVIDENCE_DIR'])
u=urllib.parse.urlsplit(os.environ['DATABASE_URL'])._replace(path='/dob_bulk_test_20261003')
assert 'bulk_test' in u.path
admin=urllib.parse.urlunsplit(u)
schema='role_process_'+uuid.uuid4().hex
work=b/schema;work.mkdir(mode=0o700)
def sql(dsn,query):
 parsed=urllib.parse.urlsplit(dsn);opts=urllib.parse.parse_qs(parsed.query);path=opts.pop('search_path',None)
 clean=urllib.parse.urlunsplit(parsed._replace(query=urllib.parse.urlencode(opts,doseq=True)))
 env=os.environ.copy()
 if path:env['PGOPTIONS']='-c search_path='+path[0]
 p=subprocess.run(['psql',clean,'-XAt','-v','ON_ERROR_STOP=1','-c',query],env=env,capture_output=True,text=True)
 if p.returncode:raise RuntimeError(p.stderr)
 return p.stdout.strip()
sql(admin,'CREATE SCHEMA '+schema)
q=urllib.parse.parse_qs(u.query);q['search_path']=[schema];dsn=urllib.parse.urlunsplit(u._replace(query=urllib.parse.urlencode(q,doseq=True)))
(work/'migrations').mkdir()
for src in (r/'migrations').glob('*.sql'):
 (work/'migrations'/src.name).write_text(src.read_text().replace("tc.table_schema='public'","tc.table_schema=current_schema()"))
(work/'master.key').write_text(base64.b64encode(os.urandom(32)).decode());(work/'master.key').chmod(0o600)
# A dedicated random master key and empty isolated schema; no production secrets
# or account/panel fixtures can be read by these test workers.
envfile=work/'test.env'
envfile.write_text('DATABASE_URL='+dsn+'\nMASTER_KEY_FILE='+str(work/'master.key')+'\nMASTER_KEY_VERSION=1\n')
envfile.chmod(0o600)
units={role:'dob-role-fixture-'+role+'-'+schema[-8:] for role in ['control','panels']}
def prop(role,key):
 return subprocess.check_output(['systemctl','show',units[role],'-p',key,'--value'],text=True).strip()
def run(cmd):
 p=subprocess.run(cmd,capture_output=True,text=True)
 if p.returncode:raise RuntimeError(p.stderr)
def wait(predicate,label,timeout=45):
 end=time.monotonic()+timeout
 while time.monotonic()<end:
  try:
   if predicate():return
  except Exception:pass
  time.sleep(.5)
 raise RuntimeError('fixture timeout: '+label)
facts={'at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'schema':schema,'units':units,'production_services_changed':False}
try:
 for role,unit in units.items():
  run(['systemd-run','--unit='+unit,'--property=Type=simple','--property=Restart=on-failure','--property=RestartSec=1','--property=MemoryMax=512M','--property=TasksMax=256','--property=KillMode=control-group','--property=TimeoutStopSec=10','--property=IPAddressDeny=any','--property=IPAddressAllow=127.0.0.0/8','--property=WorkingDirectory='+str(work),'--property=EnvironmentFile='+str(envfile),str(b/'worker.fixture'),'--role='+role])
 wait(lambda:sql(dsn,"SELECT count(DISTINCT kind) FROM worker_heartbeats WHERE last_seen_at>now()-interval '20 seconds' AND kind IN ('production-control','production-panels')")=='2','both roles heartbeat')
 assert sql(dsn,'SELECT count(*) FROM accounts')=='0'
 facts['before']={role:{'pid':prop(role,'MainPID'),'restarts':prop(role,'NRestarts')} for role in units}
 control_pid=facts['before']['control']['pid'];panel_pid=facts['before']['panels']['pid']
 run(['systemctl','kill','--kill-whom=main','--signal=SIGKILL',units['panels']])
 wait(lambda:prop('panels','ActiveState')=='active' and prop('panels','MainPID') not in ('0',panel_pid) and int(prop('panels','NRestarts'))>=1,'automatic panel role restart')
 assert prop('control','MainPID')==control_pid
 wait(lambda:sql(dsn,"SELECT count(*) FROM worker_heartbeats WHERE kind='production-panels' AND worker_id LIKE '%:"+prop('panels','MainPID')+"' AND last_seen_at>now()-interval '20 seconds'")=='1','replacement heartbeat')
 facts['after']={role:{'pid':prop(role,'MainPID'),'restarts':prop(role,'NRestarts')} for role in units}
 facts['heartbeat']=json.loads(sql(dsn,"SELECT jsonb_agg(t) FROM (SELECT kind,metadata->>'role' AS role,metadata->'database_pool' AS pool FROM worker_heartbeats WHERE last_seen_at>now()-interval '20 seconds' ORDER BY kind,last_seen_at)t"))
 facts['peer_survived']=True;facts['automatic_role_restart']='PASS'
 # Duplicate role fails before any module can enter the database.
 duplicate=subprocess.run([str(b/'worker.fixture'),'--role=control'],cwd=work,env={'PATH':os.environ['PATH'],'DATABASE_URL':dsn,'MASTER_KEY_FILE':str(work/'master.key')},capture_output=True,text=True,timeout=10)
 assert duplicate.returncode!=0 and 'worker role already owned' in duplicate.stderr
 facts['duplicate_process_rejected']=True;facts['status']='PASS'
finally:
 for role,unit in units.items():
  subprocess.run(['systemctl','stop',unit],capture_output=True)
  log=subprocess.run(['journalctl','-u',unit,'--no-pager','-o','cat'],capture_output=True,text=True)
  (b/('fixture-'+role+'.log')).write_text(log.stdout)
  subprocess.run(['systemctl','reset-failed',unit],capture_output=True)
 sql(admin,'DROP SCHEMA '+schema+' CASCADE')
 envfile.unlink(missing_ok=True);(work/'master.key').unlink(missing_ok=True)
 (b/'systemd-fixture-result.json').write_text(json.dumps(facts,indent=2)+'\n')
print(json.dumps(facts,indent=2))
