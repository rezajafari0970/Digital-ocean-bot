package provisioning

// SSHAdmissionCommand bounds unauthenticated SSH pressure on untouched default
// configurations. Authentication, ports, keys and established sessions are kept.
// A timed rollback remains armed until a subsequent fresh SSH verification.
func SSHAdmissionCommand() string      { return "DOB_SSH_ADMISSION_MODE=apply " + sshAdmissionScript }
func SSHAdmissionReadyCommand() string { return "DOB_SSH_ADMISSION_MODE=check " + sshAdmissionScript }

const sshAdmissionScript = `python3 - <<'PYSSHADMISSION'
import os,pathlib,stat,json,subprocess,tempfile,fcntl,hashlib,re
MAIN=pathlib.Path('/etc/ssh/sshd_config')
DROP=pathlib.Path('/etc/ssh/sshd_config.d/00-dob-admission.conf')
ROOT=pathlib.Path('/var/lib/digital-ocean-bot-ssh')
SSHD='/usr/sbin/sshd'
CTL='/usr/bin/systemctl'
RUN='/usr/bin/systemd-run'
UNIT='dob-ssh-admission-rollback'
MODE=os.environ['DOB_SSH_ADMISSION_MODE']
BODY='# Managed bounded SSH admission; authentication is unchanged.\nLoginGraceTime 30\nMaxStartups 20:30:60\nPerSourceMaxStartups 8\n'
WANT={'logingracetime':'30','maxstartups':'20:30:60','persourcemaxstartups':'8'}
DEFAULT={'logingracetime':'120','maxstartups':'10:30:100','persourcemaxstartups':'none'}
HASH=hashlib.sha256(BODY.encode()).hexdigest()
def require(ok,msg):
 if not ok:raise RuntimeError(msg)
def run(*args):return subprocess.run(args,check=True,capture_output=True,text=True,timeout=15).stdout
def effective():return dict(line.split(None,1) for line in run(SSHD,'-T').splitlines() if line.strip())
def stop_timer():
 p=subprocess.run([CTL,'stop',UNIT+'.timer',UNIT+'.service'],capture_output=True,text=True,timeout=15)
 if p.returncode and not (p.stderr.strip() and all('not loaded' in line or 'does not exist' in line for line in p.stderr.splitlines() if line.strip())):raise RuntimeError('cannot stop SSH rollback timer')

def safe(path,mode=None):
 s=path.lstat();require(stat.S_ISREG(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022,'unsafe SSH configuration file')
 if mode is not None:require(stat.S_IMODE(s.st_mode)==mode,'unsafe SSH journal permissions')
def atomic(path,data,mode=0o600):
 fd,name=tempfile.mkstemp(prefix='.dob-ssh-',dir=path.parent)
 try:
  os.fchmod(fd,mode)
  with os.fdopen(fd,'w') as f:f.write(data);f.flush();os.fsync(f.fileno())
  os.replace(name,path)
  fd=os.open(path.parent,os.O_RDONLY);os.fsync(fd);os.close(fd)
 finally:
  if os.path.exists(name):os.unlink(name)
require(os.geteuid()==0,'SSH admission requires root')
safe(MAIN)
before=effective()
if not DROP.exists() and not DROP.is_symlink():
 # Preserve operator settings and hosts without the standard global include.
 raw=MAIN.read_text()
 include=any(re.match(r'^\s*Include\s+',line,re.I) and str(DROP.parent/'*.conf') in line.split() for line in raw.splitlines())
 configured=[MAIN]+list(DROP.parent.glob('*.conf')) if DROP.parent.is_dir() else [MAIN]
 explicit=any(re.match(r'^\s*(LoginGraceTime|MaxStartups|PerSourceMaxStartups)\s+',line,re.I) for p in configured for line in p.read_text().splitlines())
 if not include or explicit or any(before.get(k)!=v for k,v in DEFAULT.items()):
  print('SSH operator policy or unsupported host preserved');raise SystemExit(0)
 if MODE=='check':raise SystemExit(1)
else:
 safe(DROP,0o600);require(DROP.read_text()==BODY,'unmanaged SSH admission file')
require(ROOT.exists() or MODE=='apply','missing SSH admission journal')
ROOT.mkdir(mode=0o700,exist_ok=True)
s=ROOT.lstat();require(stat.S_ISDIR(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o700,'unsafe SSH admission directory')
s=DROP.parent.lstat();require(stat.S_ISDIR(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022,'unsafe SSH include directory')
lock=os.open(ROOT/'lock',os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
with os.fdopen(lock,'r+') as locked:
 fcntl.flock(locked,fcntl.LOCK_EX)
 journal=ROOT/'state.json'
 def save(state,service):atomic(journal,json.dumps({'version':1,'state':state,'sha256':HASH,'service':service})+'\n')
 saved=None
 if journal.exists() or journal.is_symlink():
  safe(journal,0o600);saved=json.loads(journal.read_text())
  require(saved.get('version')==1 and saved.get('sha256')==HASH and saved.get('state') in ['PREPARING','APPLIED','COMMITTED','ROLLED_BACK'],'unknown SSH admission journal')
 if DROP.exists():
  safe(DROP,0o600);require(DROP.read_text()==BODY and saved is not None,'unowned SSH admission configuration')
  require(all(effective().get(k)==v for k,v in WANT.items()),'SSH effective admission mismatch')
  if saved['state'] in ['APPLIED','COMMITTED']:
   # This command was reached over a new SSH connection, proving reconnect.
   save('COMMITTED',saved['service'])
   stop_timer()
   print('bounded SSH admission verified');raise SystemExit(0)
 if MODE=='check':raise SystemExit(1)
 service=next((u for u in ['ssh','sshd'] if subprocess.run([CTL,'is-active','--quiet',u],capture_output=True,timeout=10).returncode==0),None)
 require(service is not None,'no active SSH service; configuration unchanged')
 before=effective()
 # Validate supported directives before publishing any configuration.
 args=[SSHD,'-T']
 for k,v in WANT.items():args+=['-o',k+'='+v]
 run(*args)
 stop_timer()
 save('PREPARING',service)
 rollback="""import pathlib,json,hashlib,subprocess,fcntl,os
root=pathlib.Path(ROOT_VALUE);drop=pathlib.Path(DROP_VALUE);journal=root/'state.json'
with (root/'lock').open('r+') as lock:
 fcntl.flock(lock,fcntl.LOCK_EX)
 s=json.loads(journal.read_text())
 if s.get('state')=='COMMITTED':raise SystemExit(0)
 if drop.exists() and not drop.is_symlink() and hashlib.sha256(drop.read_bytes()).hexdigest()==HASH_VALUE:
  drop.unlink()
  subprocess.run([SSHD_VALUE,'-t'],check=True,timeout=15)
  subprocess.run([CTL_VALUE,'reload',SERVICE_VALUE],check=True,timeout=15)
  s['state']='ROLLED_BACK'
  with journal.open('w') as f:json.dump(s,f);f.flush();os.fsync(f.fileno())
"""
 for key,value in [('ROOT_VALUE',str(ROOT)),('DROP_VALUE',str(DROP)),('HASH_VALUE',HASH),('SSHD_VALUE',SSHD),('CTL_VALUE',CTL),('SERVICE_VALUE',service)]:rollback=rollback.replace(key,repr(value))
 run(RUN,'--quiet','--collect','--unit='+UNIT,'--on-active=120s','/usr/bin/python3','-c',rollback)
 try:
  if not DROP.exists():atomic(DROP,BODY)
  run(SSHD,'-t')
  after=effective()
  require(all(after.get(k)==v for k,v in WANT.items()),'SSH include precedence rejected admission change')
  require({k:v for k,v in before.items() if k not in WANT}=={k:v for k,v in after.items() if k not in WANT},'unrelated SSH settings changed')
  run(CTL,'reload',service)
  save('APPLIED',service)
 except Exception:
  if DROP.exists() and not DROP.is_symlink() and DROP.read_text()==BODY:DROP.unlink()
  run(SSHD,'-t');run(CTL,'reload',service)
  save('ROLLED_BACK',service)
  stop_timer()
  raise
 print('bounded SSH admission applied; fresh connection verification required within 120 seconds')
PYSSHADMISSION`
