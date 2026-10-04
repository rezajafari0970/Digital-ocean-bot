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
 # Preserve unrelated fstab records and append our exact entry once.
 require(not FSTAB.is_symlink(),'unexpected fstab link')
 with FSTAB.open('r+') as f:
  fcntl.flock(f,fcntl.LOCK_EX)
  raw=f.read()
  entries=[line.split() for line in raw.splitlines() if line.strip() and not line.lstrip().startswith('#')]
  own=[line for line in entries if line[0]==str(swap)]
  if own:
   require(len(own)==1 and len(own[0])>=4 and own[0][1:4]==['none','swap','sw,nofail'],'unexpected managed fstab entry')
  else:
   f.seek(0,2);f.write(('' if not raw or raw.endswith('\n') else '\n')+str(swap)+' none swap sw,nofail 0 0\n');f.flush();os.fsync(f.fileno())
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
 assert any(line.split()==[str(root/'swap'),'none','swap','sw,nofail','0','0'] for line in pathlib.Path('/etc/fstab').read_text().splitlines())
except Exception:sys.exit(1)
PYMEMORYCHECK`
}
