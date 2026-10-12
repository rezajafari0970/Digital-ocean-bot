#!/usr/bin/env python3
"""Export immutable product blobs, never original history or operational data."""
import argparse,hashlib,json,pathlib,re,subprocess
PREFIXES=('cmd/','internal/','migrations/','web/','deploy/')
EXTENSIONS={'.go','.sql','.cjs','.js','.py','.sh','.conf','.service','.html','.css','.json'}
EXACT={'go.mod','go.sum','install.sh','deploy/datacenter-bot'}
MAPPED={'public-release/README.md':'README.md','public-release/gitignore':'.gitignore','public-release/checks.yml':'.github/workflows/checks.yml','public-release/check-source.py':'release/check-source.py','public-release/VALIDATION.json':'release/VALIDATION.json'}
def git(source,*args):return subprocess.check_output(['git','-C',str(source),*args])
def selected(name):
 if name in EXACT or name in MAPPED:return True
 if name.startswith(PREFIXES):
  if pathlib.PurePosixPath(name).suffix in EXTENSIONS or name.endswith('.sql.disabled'):return True
  raise ValueError('Unreviewed product file type: '+name)
 return False
def export(source,revision,destination,require_clean=True):
 source=pathlib.Path(source);destination=pathlib.Path(destination)
 revision=git(source,'rev-parse',revision+'^{commit}').decode().strip()
 if require_clean:
  dirty=git(source,'status','--porcelain','--untracked-files=all','--','cmd','internal','migrations','web','deploy','install.sh','go.mod','go.sum','public-release').decode()
  dirty='\n'.join(line for line in dirty.splitlines() if '__pycache__' not in pathlib.PurePosixPath(line[3:]).parts)
  if dirty.strip():raise ValueError('Commit/clean the product working tree before publication')
 if destination.exists() and any(destination.iterdir()):raise ValueError('Export destination must be empty')
 destination.mkdir(parents=True,exist_ok=True);files={}
 for entry in git(source,'ls-tree','-rz',revision).split(b'\0'):
  if not entry:continue
  meta,path=entry.split(b'\t',1);mode,kind,oid=meta.decode().split();name=path.decode()
  if not selected(name):continue
  if mode not in ('100644','100755') or kind!='blob' or not re.fullmatch(r'[A-Za-z0-9_./-]+',name) or '..' in pathlib.PurePosixPath(name).parts:raise ValueError('Unsafe export entry: '+name)
  target=MAPPED.get(name,name);raw=git(source,'cat-file','blob',oid)
  if len(raw)>5_000_000 or b'\0' in raw:raise ValueError('Unexpected binary or oversized source: '+name)
  p=destination/target;p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(raw);p.chmod(int(mode[-3:],8))
  files[target]=hashlib.sha256(raw).hexdigest()
 if not {'install.sh','README.md','go.mod','deploy/unattended.sh','release/check-source.py','release/VALIDATION.json'}.issubset(files):raise ValueError('Incomplete exported release')
 manifest={'schema':1,'files':dict(sorted(files.items()))}
 (destination/'release/FILES.json').write_text(json.dumps(manifest,indent=2)+'\n')
 print(json.dumps({'files':len(files)+1,'manifest_sha256':hashlib.sha256(json.dumps(manifest,sort_keys=True).encode()).hexdigest()}))
 return revision
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('source');p.add_argument('destination');p.add_argument('--revision',default='HEAD');p.add_argument('--allow-dirty',action='store_true');a=p.parse_args()
 try:export(a.source,a.revision,a.destination,not a.allow_dirty)
 except Exception as e:raise SystemExit(str(e))
