#!/usr/bin/env python3
"""Build pinned experimental server sources. Never installs or restarts services."""
import argparse,hashlib,json,os,pathlib,subprocess,tarfile,urllib.request,zipfile
repo=pathlib.Path(__file__).resolve().parents[1]
patches=repo/'patches/advanced-reality'
manifest=json.loads((patches/'manifest.json').read_text())
parser=argparse.ArgumentParser();parser.add_argument('--work-dir',required=True);args=parser.parse_args()
work=pathlib.Path(args.work_dir)
if not work.is_absolute() or work.exists():raise SystemExit('work-dir must be a new absolute directory')
work.mkdir(mode=0o700,parents=False)
receipts=[]
def run(name,command,cwd,env=None,required_tests=()):
 log=work/(name+'.log')
 with log.open('w') as f:p=subprocess.run(command,cwd=cwd,env=env,stdout=f,stderr=subprocess.STDOUT,timeout=900)
 item={'step':name,'exit':p.returncode}
 if required_tests:
  events=[]
  for line in log.read_text().splitlines():
   try:events.append(json.loads(line))
   except json.JSONDecodeError:pass
  passed={e.get('Test') for e in events if e.get('Action')=='pass'}
  failed=[e for e in events if e.get('Action') in ('fail','skip') and e.get('Test')]
  item.update(passed_tests=sorted(x for x in passed if x),skipped_or_failed=len(failed))
  if failed or not set(required_tests)<=passed:item['exit']=1
 receipts.append(item);(work/'build-receipt.json').write_text(json.dumps(receipts,indent=2))
 print(name,item['exit'],flush=True)
 if item['exit']:raise SystemExit('failed: '+name+'; see '+str(log))
for entry in manifest['repositories']:
 patch=patches/entry['patch']
 if hashlib.sha256(patch.read_bytes()).hexdigest()!=entry['patch_sha256']:raise SystemExit('patch hash mismatch')
 target=work/entry['name']
 run(entry['name']+'-init',['git','init','-q',str(target)],work)
 run(entry['name']+'-remote',['git','remote','add','origin',entry['url']],target)
 run(entry['name']+'-fetch',['git','fetch','--depth','1','origin',entry['commit']],target)
 run(entry['name']+'-checkout',['git','checkout','--detach','FETCH_HEAD'],target)
 if subprocess.check_output(['git','rev-parse','HEAD'],cwd=target,text=True).strip()!=entry['commit']:raise SystemExit('source commit mismatch')
 if hashlib.sha256((target/'LICENSE').read_bytes()).hexdigest()!=entry['license_sha256']:raise SystemExit('license drift')
 run(entry['name']+'-apply-check',['git','apply','--check',str(patch)],target)
 run(entry['name']+'-apply',['git','apply',str(patch)],target)
core_env=dict(os.environ,GOMAXPROCS='4',GOTOOLCHAIN='go1.27.1')
run('core-build',['go','build','-trimpath','-ldflags','-X github.com/xtls/xray-core/core.build='+manifest['revision'],'-o',str(work/'xray-are'),'./main'],work/'xray',core_env)
run('core-race',['go','test','-race','-json','-count=1','./common/realityguard'],work/'xray',core_env,('TestLimitDrainPreservesEstablished','TestCASReplayLeaseAndRestart','TestConcurrentBudgetAndAccounting','TestDeadlineAndFailure','TestDisabledAndUnscoped','TestHTTPRejectsInvalidAndLarge','TestSocketScopePermissionsAndCleanup','TestJournalBoundAndInvalidPolicy','TestUnregisteredScopeCannotApply','TestOwnedSocketCrashRecovery'))
run('reality-cancel-race',['go','test','-race','-json','-count=1','-run','TestAREServerCancellation','.'],work/'reality',core_env,('TestAREServerCancellationClosesBothSides',))
node=manifest['node'];name='node-'+node['version']+'-linux-x64.tar.xz';archive=work/name
urllib.request.urlretrieve('https://nodejs.org/dist/'+node['version']+'/'+name,archive)
if hashlib.sha256(archive.read_bytes()).hexdigest()!=node['archive_sha256']:raise SystemExit('Node archive hash mismatch')
with tarfile.open(archive) as t:t.extractall(work,filter='data')
node_bin=work/('node-'+node['version']+'-linux-x64')/'bin'
panel_env=dict(os.environ,GOMAXPROCS='4',GOTOOLCHAIN='go1.27.2',PATH=str(node_bin)+':'+os.environ['PATH'],ARE_NATIVE_CORE=str(work/'xray-are'))
run('frontend-install',['npm','ci'],work/'sanaei/frontend',panel_env)
run('frontend-build',['npm','run','build'],work/'sanaei/frontend',panel_env)
run('panel-build',['go','build','-trimpath','-o',str(work/'x-ui-are'),'.'],work/'sanaei',panel_env)
run('panel-native-race',['go','test','-race','-tags','are_native','-json','-count=1','-run','TestAdvancedReality','./internal/web/controller'],work/'sanaei',panel_env,('TestAdvancedRealityNativeCore','TestAdvancedRealityAuthScopeAndNoRestart','TestAdvancedRealityDisabledAndUnknownOutcome'))
run('panel-route-contract',['go','test','-race','-json','-count=1','-run','^TestRouteRegistryContract$','./internal/web'],work/'sanaei',panel_env,('TestRouteRegistryContract',))
client_info=next(x for x in json.loads((repo/'docs/ADVANCED_REALITY_BASELINES.json').read_text())['native_test_archives'] if '/v26.5.9/' in x['url'])
client_zip=work/'official-client.zip';urllib.request.urlretrieve(client_info['url'],client_zip)
if hashlib.sha256(client_zip.read_bytes()).hexdigest()!=client_info['sha256']:raise SystemExit('official client archive hash mismatch')
client=work/'official-client'
with zipfile.ZipFile(client_zip) as archive:client.write_bytes(archive.read('xray'))
client.chmod(0o700)
native_env=dict(core_env,ADVANCED_REALITY_TEST_XRAY=str(work/'xray-are'),ADVANCED_REALITY_TEST_CLIENT_XRAY=str(client))
run('bot-native-compatibility',['go','test','-race','-json','-count=1','./internal/advancedreality'],repo,native_env,('TestNativeXrayTLSXHTTPAndRealityRollback',))
hashes={name:hashlib.sha256((work/name).read_bytes()).hexdigest() for name in ['xray-are','x-ui-are']}
(work/'binary-hashes.json').write_text(json.dumps(hashes,indent=2))
print(json.dumps({'critical_tests_passed':True,'production_installed':False,'binaries':hashes}),flush=True)
