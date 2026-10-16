#!/usr/bin/env python3
"""Exercise the real release stage: stable guardian builds, meaningful changes detected."""
from pathlib import Path
import hashlib,io,json,os,shutil,subprocess,tarfile,tempfile
ROOT=Path(__file__).resolve().parents[1]
def run(args,**kw):return subprocess.run(args,check=True,**kw)
with tempfile.TemporaryDirectory(prefix='dob-guardian-stability-') as directory:
 root=Path(directory);src=root/'source';src.mkdir()
 raw=subprocess.check_output(['git','archive','HEAD'],cwd=ROOT)
 with tarfile.open(fileobj=io.BytesIO(raw)) as archive:archive.extractall(src,filter='data')
 # Test the candidate production stage, including uncommitted fixes.
 shutil.copyfile(ROOT/'deploy/service-topology.sh',src/'deploy/service-topology.sh')
 run(['git','init','-q'],cwd=src)
 run(['git','add','.'],cwd=src)
 def commit(message):
  run(['git','-c','user.name=Artifact test','-c','user.email=artifact-test@example.invalid','commit','-qm',message],cwd=src)
 commit('first release')
 shell=r'''
set -euo pipefail
source "$SRC/deploy/service-topology.sh"
# App binaries are irrelevant to this test. Both guardian builds invoke real Go.
go() {
 if [ "${*: -1}" = "./cmd/server-guardian" ]; then
  command go "$@"
 else
  local previous="" arg
  for arg in "$@"; do
   if [ "$previous" = "-o" ]; then printf 'unrelated app fixture\n' > "$arg";return;fi
   previous="$arg"
  done
  return 1
 fi
}
dob_stage_runtime
cp "$DOB_STAGE/bin/server-guardian-linux-amd64" "$RESULT-amd64"
cp "$DOB_STAGE/bin/server-guardian-linux-arm64" "$RESULT-arm64"
'''
 env=dict(os.environ);env.pop('GOFLAGS',None);env.update(SRC=str(src),APP=str(root/'runtime'),GOMAXPROCS='2',CGO_ENABLED='0')
 def build(label):
  revision=subprocess.check_output(['git','rev-parse','HEAD'],cwd=src,text=True).strip()
  env.update(BUILD_COMMIT=revision,BUILD_TIME=label,RESULT=str(root/label),LDFLAGS='-s -w -X github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo.Commit='+revision+' -X github.com/rezajafari0970/Digital-ocean-bot/internal/buildinfo.BuildTime='+label)
  run(['bash','-c',shell],cwd=src,env=env)
  return {arch:hashlib.sha256((root/(label+'-'+arch)).read_bytes()).hexdigest() for arch in ('amd64','arm64')}
 first=build('release-one')
 (src/'UNRELATED_RELEASE_NOTE').write_text('No guardian code or dependency change.\n')
 run(['git','add','UNRELATED_RELEASE_NOTE'],cwd=src);commit('unrelated controlplane release')
 second=build('release-two')
 print(json.dumps({'first':first,'second':second,'stable':first==second}),flush=True)
 if first!=second:raise SystemExit('FAIL: unrelated release changes guardian artifact')
 types=src/'internal/serverprotection/types.go';old=types.read_text()
 assert 'const Version = "1.0.0"' in old
 types.write_text(old.replace('const Version = "1.0.0"','const Version = "1.0.0-artifact-test"',1))
 changed=build('guardian-change')
 assert all(changed[a]!=second[a] for a in second),'real guardian change was hidden'
 print(json.dumps({'changed':changed,'meaningful_change_detected':True}),flush=True)
