package provisioning

// KernelMemoryGuardCommand stages the documented KHO workaround only for
// susceptible kernels. It never reboots; installer readiness and explicit
// serial maintenance own activation. Existing boot arguments are preserved.
func KernelMemoryGuardCommand() string           { return kernelMemoryGuard("prepare") }
func KernelMemoryGuardReadyCommand() string      { return kernelMemoryGuard("check") }
func KernelMemoryGuardAuditCommand() string      { return kernelMemoryGuard("audit") }
func KernelMemoryGuardActivationCommand() string { return kernelMemoryGuard("activation") }
func KernelMemoryRebootRequiredCommand() string {
	return "if test -f /var/run/reboot-required; then echo yes; else\n" + kernelMemoryGuard("needs-reboot") + "\nfi"
}
func kernelMemoryGuard(mode string) string {
	return "python3 - <<'PYKHOGUARD'\nMODE='" + mode + "'\n" + kernelMemoryGuardPython + "\nPYKHOGUARD"
}

const kernelMemoryGuardPython = `import fcntl,json,os,pathlib,re,shlex,stat,subprocess,tempfile
ROOT=pathlib.Path('/var/lib/digital-ocean-bot-kernel-memory')
DROP=pathlib.Path('/etc/default/grub.d/99-dob-kho.cfg')
GRUB=pathlib.Path('/boot/grub/grub.cfg')
BOOT=pathlib.Path('/proc/sys/kernel/random/boot_id')
CMDLINE=pathlib.Path('/proc/cmdline')
MEMINFO=pathlib.Path('/proc/meminfo')
KERNEL=pathlib.Path('/proc/sys/kernel/osrelease')
CONFIG=pathlib.Path('/boot/config-'+KERNEL.read_text().strip())
OWNER_UID=0
EXPECTED='# Managed by Digital-ocean-bot: Ubuntu phantom CMA / KHO mitigation.\nGRUB_CMDLINE_LINUX="${GRUB_CMDLINE_LINUX} kho=off"\n'
def require(ok,message):
 if not ok: raise RuntimeError(message)
def safe(path,directory=False):
 require(not path.is_symlink(),'symlink refused: '+str(path))
 s=path.stat()
 require(s.st_uid==OWNER_UID and not s.st_mode&0o022,'unsafe ownership/mode: '+str(path))
 require(stat.S_ISDIR(s.st_mode) if directory else stat.S_ISREG(s.st_mode),'unexpected file type: '+str(path))
def atomic(path,data,mode=0o600):
 if path.exists() or path.is_symlink():safe(path)
 fd,tmp=tempfile.mkstemp(dir=path.parent,prefix='.dob-kho-')
 try:
  os.fchmod(fd,mode)
  with os.fdopen(fd,'w') as f:f.write(data);f.flush();os.fsync(f.fileno())
  os.replace(tmp,path)
  fd=os.open(path.parent,os.O_RDONLY|os.O_DIRECTORY)
  try:os.fsync(fd)
  finally:os.close(fd)
 finally:
  if os.path.exists(tmp):os.unlink(tmp)
def memory():
 return {x.split(':')[0]:int(x.split()[1]) for x in MEMINFO.read_text().splitlines()}
def boot_entries():
 safe(GRUB)
 return [shlex.split(x.strip()) for x in GRUB.read_text().splitlines() if re.match(r'^\s*(linux|linuxefi|linux16)\s',x)]
def valid_grub():
 entries=boot_entries()
 return bool(entries) and all([x for x in row if x.startswith('kho=')]==['kho=off'] for row in entries)
def unrelated_entries():
 return [[x for x in row if not x.startswith('kho=')] for row in boot_entries()]
def prepared():
 if not ROOT.exists():return False
 safe(ROOT,True)
 state=ROOT/'state.json'
 if not state.exists():return False
 safe(state);v=json.loads(state.read_text())
 require(v.get('version')==1 and v.get('phase') in ['PREPARING','PREPARED'],'unknown mitigation journal')
 safe(DROP)
 return v['phase']=='PREPARED' and DROP.read_text()==EXPECTED and valid_grub()
mem=memory();kernel=KERNEL.read_text().strip();boot=BOOT.read_text().strip()
cfg=CONFIG.read_text() if CONFIG.is_file() else ''
flags=CMDLINE.read_text().split();options=[x.split('=',1)[1] for x in flags if x.startswith('kho=')]
off=bool(options) and options[-1] in ['off','0','n','no','false']
require(len(options)<=1,'conflicting duplicate KHO boot options')
supported='CONFIG_KEXEC_HANDOVER=y' in cfg
kho_default='CONFIG_KEXEC_HANDOVER_ENABLE_DEFAULT=y' in cfg
anomaly=mem.get('CmaFree',0)>mem.get('CmaTotal',0)
affected=supported and (kernel.startswith('7.0.') and kho_default or anomaly)
if MODE in ['needs-reboot','activation']:
 pending=affected and (not off or anomaly)
 journal=ROOT/'state.json'
 changed=False
 if journal.exists():
  safe(ROOT,True);safe(journal);v=json.loads(journal.read_text())
  require(v.get('version')==1,'unknown activation journal')
  changed=v.get('boot_id_before')!=boot
  pending=pending or v.get('phase')!='PREPARED' or not(changed or v.get('initially_disabled',False))
 if MODE=='needs-reboot':print('yes' if pending else 'no')
 else:
  require(not pending and off and not anomaly,'kernel mitigation not active')
  print(json.dumps(dict(active=True,boot_id=boot,boot_changed=changed,kernel=kernel,cma_total_kib=mem.get('CmaTotal',0),cma_free_kib=mem.get('CmaFree',0))))
 raise SystemExit(0)
if MODE=='audit':
 print(json.dumps(dict(kernel=kernel,boot_id=boot,affected=affected,kho_disabled=off,cma_total_kib=mem.get('CmaTotal',0),cma_free_kib=mem.get('CmaFree',0),prepared=prepared() if ROOT.exists() else False)))
 raise SystemExit(0)
if not affected and not ROOT.exists():
 print('kernel memory guard: not applicable')
 raise SystemExit(0)
if MODE=='check':
 require(prepared(),'kernel memory guard not prepared')
 print('kernel memory guard: prepared')
 raise SystemExit(0)
require(MODE=='prepare','unsupported guard mode')
require(os.geteuid()==OWNER_UID,'kernel memory guard requires root')
require(not options or off,'explicit KHO enablement requires operator review')
require(supported or ROOT.exists(),'kernel capability not confirmed')
safe(DROP.parent,True);safe(GRUB)
for candidate in [DROP.parent.parent/'grub',*DROP.parent.glob('*.cfg')]:
 if candidate==DROP or not candidate.is_file():continue
 for line in candidate.read_text().splitlines():
  if line.lstrip().startswith('#'):continue
  values=re.findall(r'kho=([a-zA-Z0-9]+)',line)
  require(not values,'existing unmanaged KHO boot setting requires review')
# Never overwrite an operator-owned drop-in, even after an interrupted run.
if DROP.exists() or DROP.is_symlink():
 safe(DROP);require(DROP.read_text()==EXPECTED,'unmanaged KHO drop-in refused')
if ROOT.exists() or ROOT.is_symlink():safe(ROOT,True)
else:ROOT.mkdir(mode=0o700)
safe(ROOT,True)
lock=ROOT/'lock'
fd=os.open(lock,os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
with os.fdopen(fd,'a+') as f:
 fcntl.flock(f,fcntl.LOCK_EX)
 state=ROOT/'state.json'
 if state.exists():
  safe(state);v=json.loads(state.read_text())
  require(v.get('version')==1 and v.get('phase') in ['PREPARING','PREPARED'],'unknown mitigation journal')
 else:
  v={'version':1,'phase':'PREPARING','boot_id_before':boot,'kernel_before':kernel,'initially_disabled':off and not anomaly,'boot_entries_before':unrelated_entries()}
  atomic(state,json.dumps(v)+'\n')
 backup=ROOT/'grub.before'
 if not backup.exists():atomic(backup,GRUB.read_text())
 else:safe(backup)
 if not DROP.exists():atomic(DROP,EXPECTED,0o644)
 if not valid_grub():
  v['phase']='PREPARING';atomic(state,json.dumps(v)+'\n')
  completed=subprocess.run(['update-grub'],capture_output=True,text=True,timeout=90)
  # Read-back resolves a lost/nonzero response; never reboot on unchecked output.
  require(valid_grub(),'generated GRUB entries do not confirm kho=off')
  require(unrelated_entries()==v['boot_entries_before'],'unrelated boot entry changed during generation')
 v['phase']='PREPARED';v['boot_id_prepared']=boot
 atomic(state,json.dumps(v)+'\n')
 require(prepared(),'mitigation read-back failed')
 print(json.dumps(dict(prepared=True,reboot_required=not off or anomaly,boot_id=boot,kernel=kernel,cma_total_kib=mem.get('CmaTotal',0),cma_free_kib=mem.get('CmaFree',0))))
`
