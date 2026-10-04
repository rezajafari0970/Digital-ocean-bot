package provisioning

// MemoryHeadroomCommand adds at most 1 GiB on a small Linux host without
// sufficient swap. It never replaces an existing swap file or disables swap.
// A root-only journal and fresh /proc/swaps reads recover interrupted activation.
func MemoryHeadroomCommand() string {
	return `set -eu
python3 - <<'PYMEMORY'
import os,pathlib,stat,fcntl,json,subprocess,tempfile
ROOT=pathlib.Path('/var/lib/digital-ocean-bot-memory')
MEM=pathlib.Path('/proc/meminfo')
SWAPS=pathlib.Path('/proc/swaps')
FSTAB=pathlib.Path('/etc/fstab')
SIZE=1024**3
def require(ok,message):
 if not ok:raise RuntimeError(message)
def memory():
 return {line.split(':')[0]:int(line.split()[1]) for line in MEM.read_text().splitlines()}
def active(path):
 return any(line.split() and line.split()[0]==str(path) for line in SWAPS.read_text().splitlines()[1:])
def safe_file(path):
 s=path.lstat()
 require(stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600,'unsafe managed memory file')
def run(*args):
 return subprocess.run(args,check=True,capture_output=True,text=True).stdout
require(os.geteuid()==0,'memory headroom requires root')
m=memory()
if not ROOT.exists() and not ROOT.is_symlink() and (m['MemTotal']>=768*1024 or m.get('SwapTotal',0)>=512*1024):
 print('memory headroom already sufficient')
 raise SystemExit(0)
# Unsupported/missing fstab must fail before activating swap.
fd=os.open(FSTAB,os.O_RDONLY|os.O_NOFOLLOW)
try:
 fs=os.fstat(fd)
 require(stat.S_ISREG(fs.st_mode) and fs.st_uid==0 and not fs.st_mode&0o022,'unsafe fstab')
finally:os.close(fd)
ROOT.mkdir(mode=0o700,exist_ok=True)
st=ROOT.lstat()
require(stat.S_ISDIR(st.st_mode) and st.st_uid==0 and stat.S_IMODE(st.st_mode)==0o700,'unsafe memory directory')
lockfd=os.open(ROOT/'lock',os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
with os.fdopen(lockfd,'r+') as lock:
 fcntl.flock(lock,fcntl.LOCK_EX)
 journal=ROOT/'state.json'
 swap=ROOT/'swap'
 def save(state):
  fd,name=tempfile.mkstemp(prefix='state-',dir=ROOT)
  try:
   with os.fdopen(fd,'w') as f:
    json.dump({'version':1,'size':SIZE,'state':state},f);f.flush();os.fsync(f.fileno())
   os.replace(name,journal)
   fd=os.open(ROOT,os.O_RDONLY);os.fsync(fd);os.close(fd)
  finally:
   if os.path.exists(name):os.unlink(name)
 if journal.exists() or journal.is_symlink():
  safe_file(journal)
  saved=json.loads(journal.read_text())
  require(saved.get('version')==1 and saved.get('size')==SIZE and saved.get('state') in ['PREPARING','FORMATTED','ACTIVE'],'unknown memory journal')
 else:
  require(not swap.exists() and not swap.is_symlink(),'unmanaged swap file exists')
  available=os.statvfs(ROOT)
  require(available.f_bavail*available.f_frsize>=2*1024**3,'insufficient disk for bounded memory headroom')
  save('PREPARING')
 if swap.exists() or swap.is_symlink():
  safe_file(swap)
  require(swap.stat().st_size<=SIZE,'unexpected swap size')
 else:
  fd=os.open(swap,os.O_CREAT|os.O_EXCL|os.O_WRONLY|os.O_NOFOLLOW,0o600);os.close(fd)
 if not active(swap):
  safe_file(swap)
  kind=subprocess.run(['blkid','-p','-o','value','-s','TYPE',str(swap)],capture_output=True,text=True)
  if kind.returncode==0:
   require(kind.stdout.strip()=='swap' and swap.stat().st_size==SIZE,'unexpected managed file format')
  else:
   require(kind.returncode==2,'cannot inspect managed swap format')
   available=os.statvfs(ROOT)
   require(available.f_bavail*available.f_frsize>=1024**3+max(0,SIZE-swap.stat().st_blocks*512),'insufficient disk for bounded memory headroom')
   fd=os.open(swap,os.O_RDWR|os.O_NOFOLLOW)
   try:os.posix_fallocate(fd,0,SIZE);os.fsync(fd)
   finally:os.close(fd)
   run('mkswap',str(swap))
  save('FORMATTED')
  run('swapon',str(swap))
 require(active(swap),'swap activation not observed')
 safe_file(swap)
 require(swap.stat().st_size==SIZE,'active swap size changed')
 # Replace the complete fstab atomically; a crash cannot leave a partial line.
 fd=os.open(FSTAB,os.O_RDONLY|os.O_NOFOLLOW)
 with os.fdopen(fd,'r') as f:
  fcntl.flock(f,fcntl.LOCK_EX)
  before=os.fstat(f.fileno())
  require(stat.S_ISREG(before.st_mode) and before.st_uid==0 and not before.st_mode&0o022,'unsafe fstab')
  raw=f.read()
  entries=[line.split() for line in raw.splitlines() if line.strip() and not line.lstrip().startswith('#')]
  desired=[str(swap),'none','swap','sw,nofail','0','0']
  own=[line for line in entries if line[0]==str(swap)]
  if own:
   require(own==[desired],'unexpected managed fstab entry')
  else:
   new=raw+('' if not raw or raw.endswith('\n') else '\n')+' '.join(desired)+'\n'
   fd,name=tempfile.mkstemp(prefix='.dob-fstab-',dir=FSTAB.parent)
   try:
    os.fchmod(fd,stat.S_IMODE(before.st_mode));os.fchown(fd,before.st_uid,before.st_gid)
    with os.fdopen(fd,'w') as updated:
     updated.write(new);updated.flush();os.fsync(updated.fileno())
    current=FSTAB.lstat()
    require((current.st_dev,current.st_ino,current.st_mtime_ns,current.st_size)==(before.st_dev,before.st_ino,before.st_mtime_ns,before.st_size),'fstab changed concurrently')
    os.replace(name,FSTAB)
    fd=os.open(FSTAB.parent,os.O_RDONLY);os.fsync(fd);os.close(fd)
   finally:
    if os.path.exists(name):os.unlink(name)
 save('ACTIVE')
 print('bounded memory headroom verified')
PYMEMORY`
}

func MemoryHeadroomReadyCommand() string {
	return `python3 - <<'PYMEMORYCHECK'
import pathlib,json,sys,os,stat
root=pathlib.Path('/var/lib/digital-ocean-bot-memory')
mem={line.split(':')[0]:int(line.split()[1]) for line in pathlib.Path('/proc/meminfo').read_text().splitlines()}
if not root.exists() and not root.is_symlink():
 sys.exit(0 if mem['MemTotal']>=768*1024 or mem.get('SwapTotal',0)>=512*1024 else 1)
try:
 s=root.lstat()
 assert stat.S_ISDIR(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o700
 for p in [root/'state.json',root/'swap']:
  s=p.lstat()
  assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600
 state=json.loads((root/'state.json').read_text())
 assert state=={'version':1,'size':1024**3,'state':'ACTIVE'}
 assert (root/'swap').stat().st_size==1024**3
 assert any(line.split() and line.split()[0]==str(root/'swap') for line in pathlib.Path('/proc/swaps').read_text().splitlines()[1:])
 fstab=pathlib.Path('/etc/fstab')
 s=fstab.lstat()
 assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022
 fd=os.open(fstab,os.O_RDONLY|os.O_NOFOLLOW)
 with os.fdopen(fd,'r') as f:entries=[line.split() for line in f.read().splitlines() if line.strip() and not line.lstrip().startswith('#')]
 assert [line for line in entries if line[0]==str(root/'swap')]==[[str(root/'swap'),'none','swap','sw,nofail','0','0']]
except Exception:sys.exit(1)
PYMEMORYCHECK`
}
