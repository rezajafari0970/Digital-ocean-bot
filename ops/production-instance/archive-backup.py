from pathlib import Path
import os,subprocess,json,fcntl,secrets,runpy,time
os.umask(0o077);os.environ['PATH']='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'
root=Path('/var/backups/dob-production-daily');repo=Path('/var/lib/dob-production-vault');password=Path('/etc/dob-production/backup-password')
with open('/var/lib/dob-production-backup.lock','a') as lock:
 fcntl.flock(lock,fcntl.LOCK_EX)
 if not password.exists():password.write_text(secrets.token_urlsafe(48)+'\n');password.chmod(0o600)
 helpers=runpy.run_path('/opt/dob-production/support/backup_store.py')
 candidates=sorted(p for p in root.iterdir() if p.is_dir() and helpers['STAMP'].fullmatch(p.name));assert candidates,'no sealed backup'
 latest=candidates[-1];assert helpers['verified'](latest),'backup manifest invalid'
 env=dict(os.environ,RESTIC_REPOSITORY=str(repo),RESTIC_PASSWORD_FILE=str(password))
 if not (repo/'config').exists():
  assert not repo.exists() or not any(repo.iterdir()),'unrecognized repository';repo.mkdir(mode=0o700,exist_ok=True);subprocess.run(['restic','init'],env=env,check=True)
 assets=[str(p) for p in Path('/etc/systemd/system').glob('dob-production*') if p.is_file() or p.is_dir()]
 snap=subprocess.run(['restic','backup','--json','--tag','dob-production','--host','production-controller','--exclude','/etc/dob-production/deployment.pending',str(latest),'/etc/dob-production','/usr/local/libexec/dob-production','/usr/local/sbin/dob-production']+assets,env=env,check=True,capture_output=True,text=True,timeout=1800)
 summary=next(json.loads(l) for l in reversed(snap.stdout.splitlines()) if json.loads(l).get('message_type')=='summary')
 subprocess.run(['restic','check'],env=env,check=True,timeout=1800)
 # Bound this dedicated encrypted repository independently of changing dump paths.
 subprocess.run(['restic','forget','--host','production-controller','--tag','dob-production','--group-by','host,tags','--keep-last','3','--keep-daily','3','--prune'],env=env,check=True,timeout=1800)
 subprocess.run(['restic','check'],env=env,check=True,timeout=1800)
 status={'ok':True,'checked_at':time.time(),'snapshot_id':summary['snapshot_id'],'source':latest.name,'repository_id':json.loads((repo/'config').read_text()) if False else 'encrypted-local-source','off_host_copy_required':True}
 Path('/var/lib/dob-production-health/archive.json').write_text(json.dumps(status,indent=2))
 # Keep three complete local dumps; off-host encrypted history is separately retained.
 helpers['prune'](root,keep=3)
 print(json.dumps(status))
