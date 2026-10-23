#!/usr/bin/env python3
"""Fresh sealed backup for this instance; no repair and no integrity bypass."""
from pathlib import Path
import datetime,fcntl,json,os,runpy,shutil,subprocess,tempfile,time
ROOT=Path("/var/backups/dob-production-daily")
ETC=Path("/etc/dob-production")
HEALTH=Path("/var/lib/dob-production-health/integrity.json")
def publish(value):
 fd,name=tempfile.mkstemp(prefix=".backup-",dir=HEALTH.parent)
 try:
  with os.fdopen(fd,"w") as f:
   os.fchmod(f.fileno(),0o644);json.dump(value,f);f.flush();os.fsync(f.fileno())
  os.replace(name,HEALTH)
  fd=os.open(HEALTH.parent,os.O_DIRECTORY)
  try:os.fsync(fd)
  finally:os.close(fd)
 finally:
  if os.path.exists(name):os.unlink(name)
def main():
 os.umask(0o077)
 os.environ["PATH"]="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
 with open("/var/lib/dob-production-backup.lock","a") as lock:
  fcntl.flock(lock,fcntl.LOCK_EX)
  result=json.loads(HEALTH.read_text())
  if result.get("status")!="ok" or result.get("recovery_required") or time.time()-result["checked_at"]>600:
   raise RuntimeError("fresh_integrity_inspection_required")
  helper=runpy.run_path("/opt/dob-production/support/backup_store.py")
  config=runpy.run_path("/opt/dob-production/support/bootstrap.py")
  env=config["database_environment"](config["read_environment"](ETC/"env"))
  ROOT.mkdir(mode=0o700,parents=True,exist_ok=True)
  candidates=sorted(p for p in ROOT.iterdir() if p.is_dir() and helper["STAMP"].fullmatch(p.name))
  latest=candidates[-1] if candidates else None
  try:
   fresh=latest is not None and time.time()-datetime.datetime.strptime(latest.name,"%Y%m%dT%H%M%S.%fZ").replace(tzinfo=datetime.timezone.utc).timestamp()<300
   if not fresh or not helper["verified"](latest):
    size=int(subprocess.check_output(["psql","-XqAt","-v","ON_ERROR_STOP=1","-c","SELECT pg_database_size(current_database())"],env=env,text=True,timeout=30))
    if shutil.disk_usage(ROOT).free<max(1024**3,int(size*1.1)+512*1024**2):
     raise RuntimeError("backup_disk_space_low")
    pending=Path(tempfile.mkdtemp(prefix=".pending-",dir=ROOT))
    with (pending/"database.dump").open("wb") as out:
     subprocess.run(["pg_dump","--format=custom","--no-owner"],env=env,stdout=out,stderr=subprocess.DEVNULL,check=True,timeout=1800)
    subprocess.run(["pg_restore","--file=/dev/null",str(pending/"database.dump")],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,check=True,timeout=1800)
    shutil.copytree(ETC,pending/"configuration",symlinks=True,ignore=shutil.ignore_patterns("deployment.pending"))
    latest=helper["seal"](pending,ROOT/datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%S.%fZ"))
   result["backup"]=dict(ok=True,checked_at=time.time(),source=latest.name)
   publish(result)
   print(json.dumps(dict(backup_ok=True,source=latest.name,reused_recent=fresh)))
  except Exception:
   result["backup"]=dict(ok=False,checked_at=time.time());publish(result);raise
if __name__=="__main__":main()
