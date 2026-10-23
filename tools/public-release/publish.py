#!/usr/bin/env python3
"""Private host publisher. Configuration/SSH credentials never enter exported source."""
import fcntl,hashlib,json,os,pathlib,runpy,shutil,subprocess,sys,tarfile,tempfile,time
BASE=pathlib.Path('/var/lib/datacenter-publisher')
HERE=pathlib.Path(__file__).resolve().parent
def run(args,**kw):
 try:return subprocess.run([str(x) for x in args],check=True,**kw)
 except subprocess.CalledProcessError as e:raise RuntimeError('Gate '+pathlib.Path(str(args[0])).name+' failed (exit '+str(e.returncode)+'); see candidate evidence') from None
def output(args,**kw):return subprocess.check_output([str(x) for x in args],**kw).decode().strip()
def write_state(data):
 temp=BASE/'state.new';temp.write_text(json.dumps(data,indent=2)+'\n');temp.chmod(0o600);os.replace(temp,BASE/'state.json')
def main():
 config=json.loads(pathlib.Path('/etc/datacenter-publisher.json').read_text())
 BASE.mkdir(mode=0o755,exist_ok=True)
 with (BASE/'lock').open('w') as lock:
  try:fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
  except BlockingIOError:print('Publisher already running');return
  source=pathlib.Path(config['source']);revision=output(['git','-C',source,'rev-parse','HEAD'])
  state=json.loads((BASE/'state.json').read_text()) if (BASE/'state.json').exists() else {}
  if state.get('source_revision')==revision and state.get('status')=='published':print('No new committed source');return
  run_dir=pathlib.Path(tempfile.mkdtemp(prefix='release-',dir=BASE));run_dir.chmod(0o755)
  print('Candidate evidence: '+str(run_dir),flush=True)
  export_dir=run_dir/'export'
  runpy.run_path(str(HERE/'export.py'))['export'](source,revision,export_dir)
  export_dir.chmod(0o755)
  manifest=hashlib.sha256((export_dir/'release/FILES.json').read_bytes()).hexdigest()
  # Metadata-only private commits need no duplicate public release.
  if state.get('manifest_sha256')==manifest and state.get('status')=='published':
   state['source_revision']=revision;write_state(state);shutil.rmtree(run_dir);print('Public product source unchanged');return
  run(['python3',HERE/'private-secret-scan.py',export_dir,source/'deploy/bootstrap.py'])
  run([HERE/'gitleaks','dir',export_dir,'--redact=100','--no-banner','--ignore-gitleaks-allow','--config',HERE/'gitleaks.toml'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
  repo=run_dir/'repo'
  run(['git','clone','--quiet','--branch','main',config['remote'],repo])
  repo.chmod(0o755)
  base=output(['git','-C',repo,'rev-parse','HEAD'])
  expected=state.get('public_revision',config['initial_revision'])
  pending=state.get('pending_revision') if state.get('status')=='publishing' else None
  if pending and base==pending:
   refs=output(['git','-C',repo,'ls-remote','origin','refs/heads/main','refs/heads/stable'])
   if len(refs.splitlines())!=2 or any(not x.startswith(pending+'\t') for x in refs.splitlines()):
    raise RuntimeError('Pending publication refs disagree; refusing acknowledgement')
   state.update(status='published',public_revision=pending,source_revision=state['pending_source'],manifest_sha256=state['pending_manifest']);write_state(state)
   print('Reconciled previously acknowledged remote publication');return
  if base!=expected:raise RuntimeError('Destination changed independently; refusing to overwrite it')
  for p in repo.iterdir():
   if p.name=='.git':continue
   if p.is_dir():shutil.rmtree(p)
   else:p.unlink()
  for p in export_dir.iterdir():
   if p.is_dir():shutil.copytree(p,repo/p.name)
   else:shutil.copy2(p,repo/p.name)
  for item in repo.rglob('*'):
   if item.is_dir():item.chmod(0o755)
   elif item.is_file():item.chmod(0o755 if item.stat().st_mode&0o111 else 0o644)
  for key,value in [('user.name','Datacenter Release Bot'),('user.email','datacenter-release@users.noreply.github.com'),('core.hooksPath','/dev/null')]:run(['git','-C',repo,'config',key,value])
  run(['git','-C',repo,'add','-A']);run(['python3',repo/'release/check-source.py'])
  run(['git','-C',repo,'commit','--quiet','-m','release: verified datacenter source update'])
  public=output(['git','-C',repo,'rev-parse','HEAD'])
  # All candidate tests run without access to root's keys, configuration or home.
  cache=pathlib.Path('/var/cache/datacenter-publisher-tests')
  script=run_dir/'validate.sh'
  # Copy only the credential-free gate into candidate evidence; HERE stays private.
  regression_gate=run_dir/'transport-regression-gate.py'
  shutil.copyfile(HERE/'transport-regression-gate.py',regression_gate);regression_gate.chmod(0o644)
  script.write_text('set -euo pipefail\ncd "$1"\npython3 "$2" "$1"\ngo test ./...\ngo vet ./...\npython3 deploy/bootstrap_test.py\npython3 deploy/project_update_test.py\npython3 deploy/host_protection_test.py\nnode --check web/static/app.js\nnode --check web/static/panel-settings.js\nnode --test web/tests/output_live.cjs\nbash -n install.sh\nfor f in deploy/*.sh deploy/datacenter-bot; do bash -n "$f"; done\n')
  script.chmod(0o755)
  with (run_dir/'validation.log').open('w') as log:
   run(['systemd-run','--quiet','--wait','--pipe','--collect',
     '-p','User=datacenter-build','-p','Group=datacenter-build','-p','ProtectHome=true','-p','ProtectSystem=strict','-p','PrivateTmp=true','-p','NoNewPrivileges=true','-p','MemoryMax=1800M','-p','CPUQuota=150%','-p','ReadWritePaths='+str(cache),
     '/usr/bin/env','-i','PATH=/usr/local/lib/datacenter-bot/go1.27.1/bin:/usr/bin:/bin','HOME='+str(cache),'GOCACHE='+str(cache/'build'),'GOPATH='+str(cache/'go'),'GOMAXPROCS=2','GOTOOLCHAIN=local','GOFLAGS=-buildvcs=false','PYTHONDONTWRITEBYTECODE=1','bash',script,repo,regression_gate],stdout=log,stderr=subprocess.STDOUT,timeout=1800)
  # Exact committed candidate is installed into a private, unprivileged LXC test container.
  container=config['test_container'];archive=run_dir/'candidate.tar.gz'
  with tarfile.open(archive,'w:gz') as tar:tar.add(repo,arcname=public)
  run(['lxc','file','push',HERE/'smoke.py',container+'/root/release-smoke.py'])
  run(['lxc','exec',container,'--','python3','/root/release-smoke.py','--before'])
  run(['lxc','file','push',archive,container+'/root/release-candidate.tar.gz'])
  run(['lxc','exec',container,'--','mkdir','-p','/var/lib/datacenter-bot-installer/revisions'])
  run(['lxc','exec',container,'--','tar','-xzf','/root/release-candidate.tar.gz','-C','/var/lib/datacenter-bot-installer/revisions'])
  target='/var/lib/datacenter-bot-installer/revisions/'+public
  # Reproduce a real root Git clone under installer umask 077; tar's
  # readable build-validation modes must not hide runtime permission bugs.
  restrictive = "import pathlib,subprocess,sys;root=pathlib.Path(sys.argv[1]);records=subprocess.check_output(['git','-C',str(root),'ls-files','-s','-z']).split(b'\\0');[(root/rec.split(b'\\t',1)[1].decode()).chmod(0o700 if rec.startswith(b'100755 ') else 0o600) for rec in records if rec];[p.chmod(0o700) for p in root.rglob('*') if p.is_dir()];root.chmod(0o700);assert (root/'web/static').stat().st_mode&0o777==0o700;assert (root/'web/static/index.html').stat().st_mode&0o777==0o600"
  run(['lxc','exec',container,'--','python3','-c',restrictive,target])
  run(['lxc','exec',container,'--','python3',target+'/deploy/static_permissions_test.py'])
  with (run_dir/'installation.log').open('w') as log:
   run(['lxc','exec',container,'--','bash',target+'/install.sh','--source',target],stdout=log,stderr=subprocess.STDOUT,timeout=1800)
  run(['lxc','exec',container,'--','python3','/root/release-smoke.py','--revision',public])
  # Recheck actual candidate bytes and current live secret literals immediately before push.
  run(['python3',repo/'release/check-source.py'])
  run(['python3',HERE/'private-secret-scan.py',repo,source/'deploy/bootstrap.py'])
  if output(['git','-C',repo,'status','--porcelain']):raise RuntimeError('Validated source mutated')
  if not config.get('publish',False):
   write_state(dict(state,validation_revision=public,validation_source=revision,validation_manifest=manifest,validation_evidence=str(run_dir)))
   print(json.dumps({'status':'validated_not_published','revision':public,'evidence':str(run_dir)}));return
  state.update(status='publishing',pending_revision=public,pending_source=revision,pending_manifest=manifest,evidence=str(run_dir));write_state(state)
  run(['git','-C',repo,'push','--atomic','origin',public+':refs/heads/main',public+':refs/heads/stable'])
  refs=output(['git','-C',repo,'ls-remote','origin','refs/heads/main','refs/heads/stable'])
  if len(refs.splitlines())!=2 or any(not x.startswith(public+'\t')for x in refs.splitlines()):raise RuntimeError('Remote publication acknowledgement unresolved')
  state.update(status='published',source_revision=revision,public_revision=public,manifest_sha256=manifest,published_at=time.time());write_state(state)
  print(json.dumps({'status':'published','public_revision':public,'source_revision':revision,'evidence':str(run_dir)}))
if __name__=='__main__':
 try:main()
 except Exception as e:
  print('Publication blocked: '+(str(e) if isinstance(e,RuntimeError) else type(e).__name__),file=sys.stderr);sys.exit(1)
