from pathlib import Path
import os,subprocess,json,fcntl,time,tempfile,shutil
os.umask(0o077);os.environ['PATH']='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'
root=Path('/var/lib/dob-production-offhost');previous=root.with_name(root.name+'.previous')
state=Path('/var/lib/dob-production-offhost-health');state.mkdir(mode=0o700,exist_ok=True)
with open('/run/lock/dob-production-offhost.lock','a') as lock:
 fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
 if not root.exists() and previous.exists():os.replace(previous,root)
 stage=Path(tempfile.mkdtemp(prefix='dob-production-offhost.stage-',dir=root.parent))
 report={'checked_at':time.time(),'ok':False}
 try:
  ssh='ssh -oBatchMode=yes -oStrictHostKeyChecking=yes -oConnectTimeout=10 -oServerAliveInterval=15 -oServerAliveCountMax=3 -oUserKnownHostsFile=/etc/dob-production-backup/known_hosts -i /etc/dob-production-backup/key'
  args=['rsync','-a','--delete','--delay-updates','--timeout=120']
  if root.exists():args+=['--link-dest='+str(root)]
  subprocess.run(args+['-e',ssh,'root@176.124.198.111:/',str(stage)+'/'],check=True,timeout=1800)
  env=dict(os.environ,RESTIC_REPOSITORY=str(stage),RESTIC_PASSWORD_FILE='/etc/dob-production-backup/password')
  subprocess.run(['restic','check','--read-data'],env=env,check=True,timeout=1800)
  snaps=json.loads(subprocess.check_output(['restic','snapshots','--json','--tag','dob-production'],env=env,text=True,timeout=60));assert snaps,'missing production snapshot'
  latest=max(snaps,key=lambda x:x['time'])
  if previous.exists():shutil.rmtree(previous)
  if root.exists():os.replace(root,previous)
  try:os.replace(stage,root)
  except BaseException:
   if not root.exists() and previous.exists():os.replace(previous,root)
   raise
  fd=os.open(root.parent,os.O_DIRECTORY)
  try:os.fsync(fd)
  finally:os.close(fd)
  report.update(ok=True,snapshot_id=latest['id'],snapshot_time=latest['time'],encrypted=True,read_all_packs=True,previous_generation_retained=previous.exists())
 finally:
  if stage.exists():shutil.rmtree(stage)
  fd,tmp=tempfile.mkstemp(dir=state,prefix='.status-')
  with os.fdopen(fd,'w') as f:json.dump(report,f,sort_keys=True);f.flush();os.fsync(f.fileno())
  os.replace(tmp,state/'status.json')
 print(json.dumps(report))
