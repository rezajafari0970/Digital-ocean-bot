import copy, datetime as dt, importlib.util, json, os, subprocess, sys, tempfile, unittest, uuid
from pathlib import Path
from unittest.mock import patch
ROOT=Path(__file__).resolve().parents[2];SPEC=importlib.util.spec_from_file_location('pilot',ROOT/'tools/residential-stability-pilot.py');m=importlib.util.module_from_spec(SPEC);SPEC.loader.exec_module(m)
SUPPORT=ROOT/'tools/residential-pilot-support.py'
m.support=m.verified_support(SUPPORT,m.hashlib.sha256(SUPPORT.read_bytes()).hexdigest())
F=Path(__file__).parent/'fixtures'
def fixture(name):return json.loads((F/(name+'.json')).read_text())
class Clock:
 def __init__(self):self.t=m.instant(fixture('stability-snapshot')['at']);self.elapsed=0
 def now(self):return self.t
 def sleep(self,seconds):self.elapsed+=seconds;self.t+=dt.timedelta(seconds=seconds)
 def monotonic(self):return self.elapsed
class Crash(BaseException):pass
class Fake:
 def __init__(self,p,c):self.p=p;self.c=c;self.current=copy.deepcopy(p['before']);self.current['inventory']={v['panel_id']:'READY' for v in self.current['assignments']};self.receipts={};self.calls=[];self.collect_count=0;self.ineligible=0;self.crash=False;self.lost=False;self.unavailable=False;self.native_fail=0;self.native_calls=0;self.expire_during_traffic=False;self.no_restore=False;self.pulse=lambda:None
 def stage(self):pass
 def runtime(self):
  if getattr(self,"runtime_changed",False):raise m.Gate("runtime identity changed")
 def snapshot(self):
  s=self.current;t=s['profile'].get('tuning') or {};panel=s['panel'];s['at']=self.c.now().isoformat()
  if t.get('phase') in ('TESTING','RESTORING') and not self.no_restore and (t['phase']=='RESTORING' or self.c.now()>=m.instant(t['deadline'])):
   t['reason']=t.get('reason','trial deadline expired');t['phase']='RESTORED';panel['config']=copy.deepcopy(self.p['before']['profile']['spec']);panel['generation']=self.p['before']['panel']['generation']+2;panel['plan_hash']=self.p['before']['panel']['plan_hash'];s['profile']['version']+=1
  panel.update(applied_generation=panel['generation'],native_generation=panel['generation'],verified_at=s['at'],native_verified_at=s['at'],state='APPLIED')
  return copy.deepcopy(s)
 def native(self,until=None):
  self.native_calls+=1
  if self.native_calls==self.native_fail:raise m.Gate('native unavailable')
  return {'passed':True}
 def traffic(self,label,until=None):
  if label=='candidate' and self.expire_during_traffic:self.c.sleep(301)
  return fixture('traffic-baseline' if label=='baseline' else 'traffic-candidate')
 def collect(self):
  self.collect_count+=1;start=self.c.now();self.c.sleep(1);identity=str(uuid.uuid4());self.current['latest_receipt']=identity
  return {'eligible':self.collect_count!=self.ineligible,'reason':'synthetic fault' if self.collect_count==self.ineligible else '', 'evidence':{'id':identity,'manifest':'vps-chain-head-v1','context':{'panel_id':self.p['panel'],'revision':self.p['before']['invariants']['routing']['revision'],'plan':self.p['before']['panel']['plan_hash']},'collection_healthy':True,'collection_outcome':'completed','context_stable':True,'chain_verified':True,'suspects':self.p['suspects'],'controls':self.p['controls'],'observations':[{} for _ in range(36)],'started':start.isoformat(),'finished':self.c.now().isoformat()}}
 def lookup(self,q):
  if self.unavailable and self.calls:raise m.Uncertain('read unavailable')
  return copy.deepcopy(self.receipts.get(q['request_id']))
 def execute(self,q):
  self.calls.append(copy.deepcopy(q));s=self.current
  if q['action']=='tune_start':
   s['profile']['version']+=1;s['profile']['tuning']={'id':q['request_id'],'phase':'TESTING','rejected':False,'panels':[self.p['panel']],'before':copy.deepcopy(self.p['before']['profile']['spec']),'candidate':q['config'],'deadline':(self.c.now()+dt.timedelta(minutes=5)).isoformat(),'admission_plan':'candidate-plan','admission_evidence_id':q['admission_evidence_id'],'base_version':q['expected_version'],'base_revision':q['base_revision']};s['panel'].update(config=q['config'],generation=self.p['before']['panel']['generation']+1,plan_hash='candidate-plan')
  else:s['profile']['tuning']['phase']='RESTORING';s['profile']['tuning']['reason']='operator cancelled';s['profile']['version']+=1
  self.receipts[q['request_id']]={'response':{'experiment_id':q['experiment_id'],'version':s['profile']['version']},'evidence_id':q.get('admission_evidence_id'),'exact_request_verified':True}
  if self.crash and q['action']=='tune_start':self.crash=False;raise Crash()
  if self.lost:raise m.Uncertain('response lost')
  return 0
class WorkflowTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.d=Path(self.tmp.name);self.c=Clock();s=fixture('stability-snapshot');s['panel']['expires_at']=(self.c.now()+dt.timedelta(hours=2)).isoformat();self.p={'schema':1,'job_id':str(uuid.uuid4()),'panel':s['panel']['id'],'suspects':['f9d1b979-53ad-4ff3-8c37-551e2003c2d1'],'controls':['343e38f3-e7cf-48fc-b478-c5c9e1f94b82','e67fad8c-ca6c-4f0a-9021-e46218cdd93c'],'created_at':self.c.now().isoformat(),'not_after':(self.c.now()+dt.timedelta(minutes=20)).isoformat(),'before':s,'tools':{n:{'path':'/reviewed/'+n,'sha256':'a'*64} for n in m.TOOLS},'runtime':{}}
  self.a=Fake(self.p,self.c)
 def tearDown(self):self.tmp.cleanup()
 def engine(self):return m.Engine(self.p,self.d,self.a,self.c.now,self.c.sleep,self.c.monotonic)
 def run_engine(self):
  with patch('builtins.print'):return self.engine().run()
 def test_two_windows_one_start_exact_restore(self):
  result=self.run_engine();self.assertEqual(result['phase'],'COMPLETE');self.assertEqual(result['trial_outcome'],'accepted');self.assertEqual(result['restoration_outcome'],'verified');self.assertEqual(self.a.collect_count,2);self.assertEqual([q['action'] for q in self.a.calls],['tune_start']);self.assertEqual(self.a.calls[0]['minutes'],5)
 def test_ineligible_window_never_retries_or_mutates(self):
  for n in (1,2):
   with self.subTest(n=n):
    if (self.d/'state.json').exists():(self.d/'state.json').unlink()
    self.a=Fake(self.p,self.c);self.a.ineligible=n;self.p['not_after']=(self.c.now()+dt.timedelta(minutes=20)).isoformat();self.p['created_at']=self.c.now().isoformat()
    result=self.run_engine();self.assertEqual(result['phase'],'NO_ACTION');self.assertEqual(len(self.a.calls),0);self.assertEqual(self.a.collect_count,n);self.run_engine();self.assertEqual(self.a.collect_count,n)
 def test_lost_reply_reconciled_without_second_start(self):
  self.a.lost=True;result=self.run_engine();self.assertEqual(result['phase'],'COMPLETE');self.assertEqual(len([q for q in self.a.calls if q['action']=='tune_start']),1)
 def test_crash_after_commit_resumes_recovery_only(self):
  self.a.crash=True
  with self.assertRaises(Crash),patch('builtins.print'):self.engine().run()
  result=self.run_engine();self.assertEqual(result['phase'],'COMPLETE');self.assertEqual(result['trial_outcome'],'rejected');self.assertEqual(self.a.collect_count,2);self.assertEqual([q['action'] for q in self.a.calls],['tune_start','tune_cancel'])
 def test_unavailable_read_does_not_claim_failure_or_repeat(self):
  self.a.unavailable=True;result=self.run_engine();self.assertEqual(result['phase'],'RECOVERY_PENDING');self.a.unavailable=False;result=self.run_engine();self.assertEqual(result['phase'],'COMPLETE');self.assertEqual(len([q for q in self.a.calls if q['action']=='tune_start']),1)
 def test_persistence_failure_prevents_submission(self):
  real=m.atomic
  def fail(path,value):
   if Path(path).name=='state.json' and value.get('phase')=='START_INTENT':raise OSError('disk failure')
   return real(path,value)
  with patch.object(m,'atomic',side_effect=fail),patch('builtins.print'),self.assertRaises(OSError):self.engine().run()
  self.assertEqual(self.a.calls,[])
 def test_native_failure_cancels_and_proves_restore(self):
  self.a.native_fail=2;result=self.run_engine();self.assertEqual(result['trial_outcome'],'rejected');self.assertEqual(result['phase'],'COMPLETE');self.assertEqual(self.a.calls[-1]['action'],'tune_cancel')
 def test_native_restore_failure_stays_pending(self):
  self.a.native_fail=3;result=self.run_engine();self.assertEqual(result['phase'],'RECOVERY_PENDING');self.assertNotEqual(result['restoration_outcome'],'verified')
 def test_expiry_during_traffic_never_counts_as_acceptance(self):
  self.a.expire_during_traffic=True;result=self.run_engine();self.assertEqual(result['phase'],'COMPLETE');self.assertEqual(result['trial_outcome'],'rejected');self.assertEqual([q['action'] for q in self.a.calls],['tune_start'])
 def test_other_owner_is_never_cancelled(self):
  self.a.crash=True
  with self.assertRaises(Crash),patch('builtins.print'):self.engine().run()
  self.a.current['profile']['tuning']['id']=str(uuid.uuid4());result=self.run_engine();self.assertEqual(result['phase'],'RECOVERY_PENDING');self.assertEqual(len(self.a.calls),1)
 def test_policy_tamper_and_request_tamper(self):
  self.a.crash=True
  with self.assertRaises(Crash),patch('builtins.print'):self.engine().run()
  altered=copy.deepcopy(self.p);altered['suspects']=['8652799c-7d5a-4a9a-8d86-7d1f519b31dc']
  with self.assertRaises(m.Gate):m.Engine(altered,self.d,self.a)
  state=json.loads((self.d/'state.json').read_text());state['request']['minutes']=30;m.atomic(self.d/'state.json',state);result=self.run_engine();self.assertEqual(result['phase'],'RECOVERY_PENDING');self.assertEqual(len(self.a.calls),1)
 def test_rehashed_request_cannot_change_frozen_trial(self):
  self.a.crash=True
  with self.assertRaises(Crash),patch('builtins.print'):self.engine().run()
  state=json.loads((self.d/'state.json').read_text());state['request']['minutes']=30;state['request_hash']=m.digest(state['request']);m.atomic(self.d/'state.json',state)
  result=self.run_engine();self.assertEqual(result['phase'],'RECOVERY_PENDING');self.assertEqual(len(self.a.calls),1)
 def test_uncommitted_ambiguous_start_replays_identical_intent_once(self):
  original=self.a.execute;attempted=[]
  def unavailable(q):
   attempted.append(copy.deepcopy(q))
   if len(attempted)==1:raise m.Uncertain('transport closed before commit')
   return original(q)
  self.a.execute=unavailable;result=self.run_engine();self.assertEqual(result['phase'],'RECOVERY_PENDING');self.assertEqual(self.a.calls,[])
  result=self.run_engine();self.assertEqual(result['phase'],'COMPLETE');self.assertEqual(attempted[0],attempted[1]);self.assertEqual(self.a.collect_count,2);self.assertEqual(len([q for q in self.a.calls if q['action']=='tune_start']),1)
 def test_mismatched_committed_receipt_never_counts_as_trial(self):
  original=self.a.lookup
  def mismatch(q):
   receipt=original(q)
   if receipt:receipt['evidence_id']=str(uuid.uuid4())
   return receipt
  self.a.lookup=mismatch;result=self.run_engine();self.assertEqual(result['phase'],'RECOVERY_PENDING');self.assertNotEqual(result['trial_outcome'],'accepted');self.assertEqual(self.a.native_calls,1)
 def test_recovery_budget_is_bounded(self):
  self.a.no_restore=True;result=self.run_engine();self.assertEqual(result['phase'],'RECOVERY_PENDING');self.assertLessEqual(self.c.elapsed,1300)
 def test_terminal_run_does_not_probe_again(self):
  self.run_engine();count=(len(self.a.calls),self.a.collect_count,self.a.native_calls);self.run_engine();self.assertEqual(count,(len(self.a.calls),self.a.collect_count,self.a.native_calls))
class EvidenceTests(unittest.TestCase):
 def test_real_safe_traffic_fixtures(self):
  self.assertEqual(m.score(fixture('traffic-baseline'))['successes'],5);self.assertEqual(m.score(fixture('traffic-candidate'))['successes'],8);self.assertTrue(m.accepted(fixture('traffic-baseline'),fixture('traffic-candidate')))
 def test_missing_duplicate_false_positive_or_nonfinite(self):
  original=fixture('traffic-candidate')
  for kind in ('missing','duplicate','local-deny','wrong-http','nan','core-dead','same-egress'):
   x=copy.deepcopy(original)
   if kind=='missing':x['cases'].pop()
   elif kind=='duplicate':x['cases'][-1]=x['cases'][0]
   elif kind=='local-deny':next(v for v in x['cases'] if v['case']=='deny-example.com')['curl_code']=7
   elif kind=='wrong-http':next(v for v in x['cases'] if v['case']=='ads-0')['http']=500
   elif kind=='nan':next(v for v in x['cases'] if v['case']=='ads-0')['seconds']=float('nan')
   elif kind=='core-dead':x['local_core_healthy']=False
   else:x['egress_distinct']=False
   with self.subTest(kind=kind),self.assertRaises(m.Gate):m.score(x)
 def test_subthreshold_or_slow_candidate_rejected(self):
  a=fixture('traffic-baseline');b=fixture('traffic-candidate');next(v for v in b['cases'] if v['case']=='ads-0').update(passed=False,http=0,curl_code=28);self.assertFalse(m.accepted(a,b));b=fixture('traffic-candidate')
  for v in b['cases']:
   if v['case'] in m.STATIC and v['passed']:v['seconds']=50
  self.assertFalse(m.accepted(a,b))
 def test_concurrent_process_lock_and_release(self):
  with tempfile.TemporaryDirectory() as d:
   p=Path(d)/'lock';code="import fcntl,os,sys; f=os.open(sys.argv[1],os.O_RDWR); fcntl.flock(f,fcntl.LOCK_EX|fcntl.LOCK_NB)"
   with m.lock(p):self.assertNotEqual(subprocess.run([sys.executable,'-c',code,str(p)],stderr=subprocess.DEVNULL).returncode,0)
   self.assertEqual(subprocess.run([sys.executable,'-c',code,str(p)]).returncode,0)
 @unittest.skipUnless(os.geteuid()==0,'operator-owned staging requires root')
 def test_pinned_recovery_tools_survive_missing_source_but_reject_changes(self):
  with tempfile.TemporaryDirectory() as d:
   root=Path(d);source=root/'source';source.write_bytes(b'reviewed artifact');source.chmod(0o500);policy={'tools':{name:{'path':str(source),'sha256':m.hashlib.sha256(source.read_bytes()).hexdigest()} for name in m.TOOLS}}
   policy['tools']['support']={'path':str(SUPPORT),'sha256':m.hashlib.sha256(SUPPORT.read_bytes()).hexdigest()};adapter=m.Adapter(policy,root);adapter.stage();source.unlink();adapter.stage()
   staged=root/'tools'/'tune';staged.chmod(0o700)
   with self.assertRaises(m.Gate):adapter.stage()
   staged.write_bytes(b'wrong artifact');staged.chmod(0o500)
   with self.assertRaises(m.Gate):adapter.stage()
 def test_real_subprocess_timeout_kills_owned_process(self):
  with tempfile.TemporaryDirectory() as d:
   root=Path(d);pidfile=root/'child.pid';adapter=m.Adapter({},root);adapter.support=m.support
   code="import os,time,sys; open(sys.argv[1],'w').write(str(os.getpid())); time.sleep(30)"
   with self.assertRaises(m.Uncertain):adapter.command('timeout-test',[sys.executable,'-c',code,str(pidfile)],.1)
   pid=int(pidfile.read_text());self.assertFalse(Path('/proc/'+str(pid)).exists())
 def test_stale_heartbeat_is_not_active(self):
  with tempfile.TemporaryDirectory() as d:
   m.atomic(Path(d)/'status.json',{'pid':os.getpid(),'pid_start_ticks':Path('/proc/self/stat').read_text().rsplit(')',1)[1].split()[19],'boot_id':Path('/proc/sys/kernel/random/boot_id').read_text().strip(),'valid_until':(m.now()-dt.timedelta(seconds=1)).isoformat()});self.assertFalse(m.status(d)['activity_confirmed'])
if __name__=='__main__':unittest.main()
