#!/usr/bin/env python3
"""Bounded, recovery-first operator workflow. No scheduler or alternate writer."""
import argparse, contextlib, datetime as dt, fcntl, hashlib, json, math, os
from pathlib import Path
import shutil, signal, stat, statistics, subprocess, sys, time, uuid
UTC=dt.timezone.utc
TEHRAN=dt.timezone(dt.timedelta(hours=3,minutes=30))
TERMINAL={'NO_ACTION','COMPLETE','ERROR'}
TOOLS={'admission','tune','native','traffic','xray'}
SERVICES={'api':'api_sha256','worker':'worker_sha256','worker-panels':'worker_sha256'}
STATIC={f'{name}-{n}' for name in ('ads','gpt','browserleaks') for n in range(3)}
CRITICAL={'probe-www.gstatic.com','probe-connectivitycheck.gstatic.com','probe-www.google.com','deny-example.com','deny-1.1.1.1','deny-dns.google','direct-positive','egress-res','egress-direct'}
class Gate(Exception): pass
class Uncertain(Exception): pass

def canonical(x): return json.dumps(x,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()
def digest(x): return hashlib.sha256(canonical(x)).hexdigest()
def instant(s): return dt.datetime.fromisoformat(s.replace('Z','+00:00')).astimezone(UTC)
def now(): return dt.datetime.now(UTC)
def require(v,message):
 if not v: raise Gate(message)
def uid(s):
 require(isinstance(s,str) and str(uuid.UUID(s))==s,'canonical UUID required'); return s

def atomic(path,value):
 path=Path(path);tmp=path.with_name(path.name+'.'+str(uuid.uuid4())+'.tmp')
 try:
  fd=os.open(tmp,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
  with os.fdopen(fd,'wb') as f:f.write(canonical(value)+b'\n');f.flush();os.fsync(f.fileno())
  os.replace(tmp,path)
  fd=os.open(path.parent,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
 finally:
  if tmp.exists():tmp.unlink()

@contextlib.contextmanager
def lock(path,shared=False):
 fd=os.open(path,os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
 try:
  fcntl.flock(fd,(fcntl.LOCK_SH if shared else fcntl.LOCK_EX)|fcntl.LOCK_NB)
  yield fd
 finally:os.close(fd)

def validate_policy(p):
 require(set(p)=={'schema','job_id','panel','suspects','controls','created_at','not_after','before','tools','runtime'},'unknown or missing policy fields')
 require(p['schema']==1,'unsupported policy schema');uid(p['job_id']);uid(p['panel'])
 ids=p['suspects']+p['controls'];require(1<=len(p['suspects'])<=2 and len(p['controls'])==2 and len(ids)==len(set(ids)),'invalid fixed proxy roles')
 for x in ids:uid(x)
 require(set(p['tools'])==TOOLS,'all pinned tools required')
 for t in p['tools'].values():require(set(t)=={'path','sha256'} and Path(t['path']).is_absolute() and len(t['sha256'])==64,'invalid tool pin')
 created=instant(p['created_at']);expiry=instant(p['not_after']);require(0<(expiry-created).total_seconds()<=1200,'qualification window must be at most20minutes')
 before=p['before'];profile=before['profile'];uid(profile['id'])
 require(profile['state']=='KEPT' and profile['duration_mode']=='permanent' and profile['publish_scope']=='fleet','permanent fleet parent required')
 require(profile['spec']['fast_count']==13 and profile['spec']['fast_share']==80 and not profile['spec'].get('excluded_proxy_ids'),'unchanged13/80 parent required')
 require(before['panel']['id']==p['panel'],'wrong frozen panel')
 return p

def score(traffic):
 cases=traffic.get('cases',[]);keys=[x.get('case') for x in cases]
 require(len(cases)==18 and set(keys)==STATIC|CRITICAL and len(set(keys))==18,'incomplete or duplicate traffic cases')
 require(traffic.get('local_core_healthy') is True and traffic.get('egress_distinct') is True,'missing core or egress proof')
 good=[]
 for x in cases:
  key=x['case'];passed=x.get('passed') is True
  if key in CRITICAL:
   require(passed,'mandatory traffic case failed')
   if key.startswith('deny-'):require(x['http']==0 and x['curl_code'] in (35,52,56),'unproven destination denial')
   else:require(x['curl_code']==0 and x['http']==(204 if key.startswith('probe-') else 200),'invalid positive traffic proof')
  elif passed:
   require(x['curl_code']==0 and x['http']==200 and isinstance(x['seconds'],(float,int)) and math.isfinite(x['seconds']) and x['seconds']>0,'invalid static success')
   good.append(x['seconds'])
 return {'successes':len(good),'median':statistics.median(good) if good else None}

def accepted(baseline,candidate):
 a=score(baseline);b=score(candidate)
 require(a['median'] is not None,'no usable baseline')
 return b['successes']>=max(8,a['successes']) and b['median'] is not None and b['median']<=max(2*a['median'],a['median']+.5)

def unchanged(p,s):
 before=p['before'];require(s['panel']['identity']==before['panel']['identity'],'panel API identity changed');require(s['invariants']==before['invariants'],'global policy or endpoint identity changed')
 require(s['profile']['id']==before['profile']['id'] and s['profile']['spec']==before['profile']['spec'],'parent ownership or configuration changed')
 old={v['panel_id']:v for v in before['assignments'] if v['panel_id']!=p['panel']};current={v['panel_id']:v for v in s['assignments'] if v['panel_id']!=p['panel']}
 for key,v in old.items():
  if key in current:require(current[key]==v,'unselected surviving assignment changed')
 for key,v in current.items():
  if key not in old:require(v['config']==before['profile']['spec'] and v['owner']==before['profile']['id'],'new unrelated assignment differs from parent')

def fresh(s):
 t=instant(s['at']);p=s['panel']
 require(p['state']=='APPLIED' and p['native_revision']==s['invariants']['routing']['revision'],'routing not applied')
 require(p['generation']==p['applied_generation']==p['native_generation'],'generation lacks native proof')
 for k in ('verified_at','native_verified_at'):require(p[k] and 0<=(t-instant(p[k])).total_seconds()<60,'stale or future native proof')

def parent_ready(p,s):
 unchanged(p,s);fresh(s);before=p['before'];x=s['profile'];t=x.get('tuning')
 require(x['state']=='KEPT' and x['duration_mode']=='permanent' and x['publish_scope']=='fleet','parent lifecycle changed')
 require(x['version']==before['profile']['version'] and (not t or t['phase'] in ('RESTORED','PUBLISHED')),'another tuning changed parent')
 require(s['panel']['owner']==x['id'] and s['panel']['config']==x['spec'] and s['panel']['generation']==before['panel']['generation'] and s['panel']['plan_hash']==before['panel']['plan_hash'],'selected assignment changed')
 require(s['panel']['server_state']=='READY' and s['panel']['enabled'] and s['panel']['account_active'],'server not eligible')
 require(not s['panel']['expires_at'] or instant(s['panel']['expires_at'])>instant(s['at'])+dt.timedelta(minutes=20),'insufficient server lifetime')

def active(p,s,request):
 unchanged(p,s);fresh(s);t=s['profile'].get('tuning') or {};q=s['panel']
 require(t.get('id')==request['request_id'] and t.get('phase')=='TESTING','trial no longer owned and testing')
 require(instant(s['at'])<instant(t['deadline']),'trial deadline expired')
 require(t['panels']==[p['panel']] and t['before']==p['before']['profile']['spec'] and t['candidate']==request['config'],'trial context changed')
 require(q['owner']==request['experiment_id'] and q['generation']==p['before']['panel']['generation']+1 and q['config']==request['config'],'candidate generation or config changed')
 require(t.get('admission_plan')==q['plan_hash'],'candidate native plan not bound')
 return t

def restored(p,s,request):
 unchanged(p,s);fresh(s);t=s['profile'].get('tuning') or {};q=s['panel']
 require(t.get('id')==request['request_id'] and t.get('phase')=='RESTORED','owned restoration remains pending')
 require(t['before']==p['before']['profile']['spec'] and q['config']==t['before'] and q['owner']==request['experiment_id'],'exact parent restoration unproved')
 require(q['generation']==p['before']['panel']['generation']+2,'unexpected restoration generation')
 return True

class Adapter:
 def __init__(self,policy,directory,pulse=lambda:None):self.p=policy;self.directory=Path(directory);self.pulse=pulse;self.env=os.environ.copy();self.tools={}
 def stage(self):
  dest=self.directory/'tools';dest.mkdir(mode=0o700,exist_ok=True);ds=dest.lstat();require(stat.S_ISDIR(ds.st_mode) and ds.st_uid==0 and not ds.st_mode&0o077,'untrusted staged directory')
  for name,spec in self.p['tools'].items():
   target=dest/name
   require(not target.is_symlink(),'staged tool symlink rejected')
   if not target.exists():
    source=Path(spec['path']);st=source.lstat();require(stat.S_ISREG(st.st_mode) and st.st_uid==0 and not st.st_mode&0o022,'untrusted tool permissions')
    data=source.read_bytes();require(hashlib.sha256(data).hexdigest()==spec['sha256'],'tool identity changed');fd=os.open(target,os.O_CREAT|os.O_EXCL|os.O_WRONLY|os.O_NOFOLLOW,0o500)
    with os.fdopen(fd,'wb') as f:f.write(data);f.flush();os.fsync(f.fileno())
    fd=os.open(dest,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
   st=target.lstat();require(stat.S_ISREG(st.st_mode) and st.st_uid==0 and not st.st_mode&0o222,'staged tool must remain read-only')
   require(hashlib.sha256(target.read_bytes()).hexdigest()==spec['sha256'],'staged tool identity changed');self.tools[name]=str(target)
 def command(self,name,args,seconds,input_text=None,env=None,until=None):
  import ctypes
  label=name+'-'+str(uuid.uuid4());out=self.directory/(label+'.stdout');fd=os.open(out,os.O_CREAT|os.O_EXCL|os.O_WRONLY,0o600);started=time.monotonic();p=None
  try:
   with os.fdopen(fd,'wb') as f:
    p=subprocess.Popen(args,stdin=subprocess.PIPE if input_text is not None else subprocess.DEVNULL,stdout=f,stderr=subprocess.DEVNULL,env=env or self.env,start_new_session=True,preexec_fn=lambda:ctypes.CDLL(None).prctl(1,signal.SIGKILL))
    if input_text is not None:p.stdin.write(input_text.encode());p.stdin.close()
    while p.poll() is None:
     self.pulse()
     if time.monotonic()-started>seconds or out.stat().st_size>2*1024*1024 or (until and now()>=until):raise Uncertain('bounded subprocess interrupted')
     time.sleep(.25)
   atomic(self.directory/(label+'.result.json'),{'tool':name,'returncode':p.returncode,'seconds':time.monotonic()-started})
   return p.returncode,out.read_text()
  finally:
   if p is not None and p.poll() is None:
    os.killpg(p.pid,signal.SIGTERM)
    try:p.wait(timeout=2)
    except subprocess.TimeoutExpired:os.killpg(p.pid,signal.SIGKILL);p.wait()
 def sql(self,query):
  env=self.env.copy();env['PGOPTIONS']='-c default_transaction_read_only=on -c statement_timeout=10000'
  rc,text=self.command('read-db',['psql',env['DATABASE_URL'],'-XAt','-v','ON_ERROR_STOP=1','-c',query],12,env=env)
  if rc:raise Uncertain('authoritative read unavailable')
  try:return json.loads(text)
  except Exception:raise Uncertain('authoritative response invalid') from None
 def snapshot(self):
  panel=uid(self.p['panel'])
  query="""SELECT json_build_object('at',clock_timestamp(),'profile',json_build_object('id',x.id,'state',x.state,'version',x.version,'spec',x.spec,'duration_mode',x.duration_mode,'publish_scope',x.publish_scope,'tuning',x.tuning),
 'panel',json_build_object('identity',md5(jsonb_build_array(pi.account_id,pi.droplet_id,pi.driver,pi.base_url,pi.auth_secret_ref)::text),'id',p.panel_id,'owner',p.experiment_id,'config',p.config,'generation',p.generation,'applied_generation',p.applied_generation,'verified_at',p.verified_at,'native_generation',r.performance_generation,'native_verified_at',r.verified_at,'native_revision',r.revision,'plan_hash',r.plan_hash,'state',r.state,'server_state',d.state,'expires_at',d.expires_at,'enabled',pi.enabled,'account_active',a.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL),
 'assignments',(SELECT COALESCE(json_agg(t ORDER BY t.panel_id),'[]'::json) FROM(SELECT pp.panel_id,pp.experiment_id AS owner,pp.config,pp.generation FROM residential_performance_panels pp JOIN panel_instances pii ON pii.id=pp.panel_id JOIN droplets dd ON dd.id=pii.droplet_id WHERE dd.state<>'DELETED')t),
 'invariants',json_build_object('routing',(SELECT row_to_json(z) FROM(SELECT enabled,fleet,panel_ids,revision FROM residential_routing_control WHERE singleton)z),'protection',(SELECT row_to_json(z) FROM(SELECT enabled,scope,revision FROM server_protection_control WHERE singleton)z),'global',md5((SELECT json_agg(z ORDER BY policy_key)::text FROM global_config_policies z)),'profiles',md5((SELECT json_agg(z ORDER BY route_class)::text FROM reality_config_profiles z)),'proxy_versions',(SELECT COALESCE(jsonb_object_agg(proxy_id::text,admission_version),'{}'::jsonb) FROM residential_proxies WHERE enabled)),
 'latest_receipt',(SELECT id FROM residential_admission_evidence WHERE panel_id=p.panel_id ORDER BY recorded_at DESC,id DESC LIMIT 1))
 FROM residential_performance_panels p JOIN residential_performance_experiments x ON x.id=p.experiment_id JOIN panel_instances pi ON pi.id=p.panel_id JOIN droplets d ON d.id=pi.droplet_id JOIN accounts a ON a.id=pi.account_id JOIN panel_routing_state r ON r.panel_id=p.panel_id WHERE p.panel_id='"""+panel+"'"
  return self.sql(query)
 def runtime(self):
  manifest=json.loads(Path('/opt/digital-ocean-bot/build-manifest.json').read_text());require(manifest==self.p['runtime'],'runtime manifest changed')
  for unit,key in SERVICES.items():
   rc,text=self.command('service',['systemctl','show','digital-ocean-bot-'+unit+'.service','-p','ActiveState','-p','MainPID'],5);v=dict(x.split('=',1) for x in text.splitlines());require(not rc and v.get('ActiveState')=='active','service not active')
   require(hashlib.sha256(Path('/proc/'+v['MainPID']+'/exe').read_bytes()).hexdigest()==manifest[key],'running service identity mismatch')
  import urllib.request
  with urllib.request.urlopen('http://127.0.0.1:18080/readyz',timeout=3) as response:ready=json.load(response)
  require(ready.get('status')=='ready' and len(ready.get('checks',[]))==5 and all(v.get('ok') is True for v in ready['checks']),'runtime readiness failed')
 def collect(self):
  rc,text=self.command('admission',[self.tools['admission'],'-probe','-panel',self.p['panel'],'-suspects',','.join(self.p['suspects']),'-controls',','.join(self.p['controls']),'-xray',self.tools['xray']],370)
  if rc:raise Gate('collector did not complete')
  try:x=json.loads(text);uid(x['evidence']['id'])
  except Exception:raise Gate('invalid collector response') from None
  # Reload the immutable database receipt; stdout alone is not authority.
  stored=self.sql("SELECT evidence FROM residential_admission_evidence WHERE id='"+x['evidence']['id']+"'")
  require(stored==x['evidence'],'collector and immutable receipt differ');return x
 def lookup(self,request):
  return self.sql("SELECT (SELECT json_build_object('response',response,'evidence_id',admission_evidence_id) FROM residential_performance_operations WHERE request_id='"+uid(request['request_id'])+"')")
 def execute(self,request):return self.command('tune',[self.tools['tune'],'--execute'],25,input_text=json.dumps(request))[0]
 def native(self,until=None):
  rc,text=self.command('native',[self.tools['native'],self.p['panel']],90,until=until)
  require(rc==0 and ('ADS_UDP_DNS_PASS panel='+self.p['panel']+' route_proofs=106') in text and 'panels=1 passed=1 pending=0' in text,'native route proof failed')
  return {'at':now().isoformat(),'passed':True,'route_proofs':106}
 def traffic(self,label,until=None):
  require(not (self.directory/(label+'.json')).exists(),'traffic window already attempted')
  rc,_=self.command('traffic',[sys.executable,self.tools['traffic'],self.p['panel'],label,str(self.directory),self.tools['xray']],80,until=until)
  file=self.directory/(label+'.json');require(file.exists(),'traffic collector incomplete')
  x=json.loads(file.read_text());require(rc in (0,1) and x.get('panel')==self.p['panel'],'traffic collector failed');return x

class Engine:
 def __init__(self,policy,directory,adapter,clock=now,pause=time.sleep,monotonic=time.monotonic):
  self.p=validate_policy(policy);self.d=Path(directory);self.a=adapter;self.clock=clock;self.pause=pause;self.mono=monotonic;self.last_pulse=-100
  f=self.d/'state.json';self.resuming=f.exists()
  self.s=json.loads(f.read_text()) if f.exists() else {'schema':1,'policy_hash':digest(policy),'phase':'NEW','trial_outcome':'not_started','restoration_outcome':'not_required','operation_attempts':{}}
  require(self.s['schema']==1 and self.s['policy_hash']==digest(policy),'persisted policy identity mismatch')
  self.a.pulse=self.pulse
 def save(self,phase,reason=''):
  self.s.update(phase=phase,reason=reason,updated_at=self.clock().isoformat());atomic(self.d/'state.json',self.s);self.pulse(force=True)
 def pulse(self,force=False):
  if not force and self.mono()-self.last_pulse<15:return
  self.last_pulse=self.mono();t=self.clock();live=self.s['phase'] not in TERMINAL|{'RECOVERY_PENDING'}
  status={'job_id':self.p['job_id'],'phase':self.s['phase'],'reason':self.s.get('reason',''),'pid':os.getpid(),'pid_start_ticks':Path('/proc/self/stat').read_text().rsplit(')',1)[1].split()[19],'boot_id':Path('/proc/sys/kernel/random/boot_id').read_text().strip(),'at_utc':t.isoformat(),'at_tehran':t.astimezone(TEHRAN).isoformat(),'valid_until':(t+dt.timedelta(seconds=90)).isoformat() if live else None,'trial_outcome':self.s['trial_outcome'],'restoration_outcome':self.s['restoration_outcome'],'background_ai':False}
  atomic(self.d/'status.json',status)
  if force:print(json.dumps(status,ensure_ascii=False),flush=True)
 def qualification(self,x):
  require(x.get('eligible') is True,'destination evidence did not qualify');e=x['evidence'];uid(e['id'])
  require(e['manifest']=='vps-chain-head-v1' and e['context']['panel_id']==self.p['panel'] and e['collection_healthy'] is True and e['collection_outcome']=='completed' and e['context_stable'] is True and e['chain_verified'] is True,'collector proof invalid')
  require(sorted(e['suspects'])==sorted(self.p['suspects']) and sorted(e['controls'])==sorted(self.p['controls']),'fixed proxy roles changed')
  require(len(e['observations'])==12*(len(self.p['suspects'])+2),'incomplete evidence')
  return e
 def prepare(self):
  self.save('PRECHECK');self.a.runtime();s=self.a.snapshot();parent_ready(self.p,s)
  require(self.clock()<instant(self.p['not_after']) and instant(self.p['created_at'])<=self.clock()+dt.timedelta(seconds=1),'qualification policy expired or future')
  self.a.native();self.save('BASELINE');self.s['baseline']=self.a.traffic('baseline');score(self.s['baseline']);parent_ready(self.p,self.a.snapshot())
  self.save('OBSERVE_FIRST');first=self.a.collect();self.s['first']=first;self.save('OBSERVE_FIRST','first receipt retained');e1=self.qualification(first)
  # Use a monotonic wait; authoritative DB gate later verifies measurement times.
  self.save('HOLD','waiting60seconds between independent windows');end=self.mono()+60
  while self.mono()<end:
   self.pulse();self.pause(min(5,max(0,end-self.mono())))
  parent_ready(self.p,self.a.snapshot());require(self.clock()<instant(self.p['not_after']),'qualification window expired')
  self.save('OBSERVE_SECOND');second=self.a.collect();self.s['second']=second;self.save('OBSERVE_SECOND','second receipt retained');e2=self.qualification(second)
  require(e1['id']!=e2['id'] and e1['context']==e2['context'],'independent contexts differ')
  require(instant(e2['started'])>=instant(e1['finished'])+dt.timedelta(seconds=60),'observation windows overlap')
  s=self.a.snapshot();parent_ready(self.p,s);t=instant(s['at'])
  require(t-instant(e1['started'])<=dt.timedelta(minutes=10) and t-instant(e2['started'])<=dt.timedelta(minutes=5) and self.clock()<instant(self.p['not_after']),'evidence expired before intent')
  require(s['latest_receipt']==e2['id'],'receipt superseded');self.a.runtime()
  cfg=dict(self.p['before']['profile']['spec']);cfg['excluded_proxy_ids']=sorted(self.p['suspects'])
  self.s['request']={'request_id':str(uuid.uuid4()),'experiment_id':s['profile']['id'],'action':'tune_start','expected_version':s['profile']['version'],'config':cfg,'panel_ids':[self.p['panel']],'minutes':5,'base_revision':e2['context']['revision'],'base_plan':e2['context']['plan'],'admission_evidence_id':e2['id'],'stability_evidence_id':e1['id']}
  self.s['request_hash']=digest(self.s['request']);self.save('START_INTENT','exact request durable before invocation')
 def contract(self,key):
  request=self.s[key];require(digest(request)==self.s[key+'_hash'],'saved request changed');uid(request['request_id'])
  if key=='request':
   first=self.qualification(self.s['first']);second=self.qualification(self.s['second']);cfg=dict(self.p['before']['profile']['spec']);cfg['excluded_proxy_ids']=sorted(self.p['suspects'])
   expected={'request_id':request['request_id'],'experiment_id':self.p['before']['profile']['id'],'action':'tune_start','expected_version':self.p['before']['profile']['version'],'config':cfg,'panel_ids':[self.p['panel']],'minutes':5,'base_revision':second['context']['revision'],'base_plan':second['context']['plan'],'admission_evidence_id':second['id'],'stability_evidence_id':first['id']}
   require(first['id']!=second['id'] and first['context']==second['context'],'saved evidence context changed')
  else:
   self.contract('request')
   require(isinstance(request['expected_version'],int) and request['expected_version']>self.p['before']['profile']['version'],'invalid cancellation version')
   expected={'request_id':request['request_id'],'experiment_id':self.p['before']['profile']['id'],'action':'tune_cancel','expected_version':request['expected_version'],'tuning_id':self.s['request']['request_id']}
  require(request==expected,'saved operation violates frozen contract')
  return request
 def confirm(self,key,receipt):
  request=self.s[key]
  require(receipt['response']['experiment_id']==request['experiment_id'],'committed operation identity mismatch')
  require(isinstance(receipt['response']['version'],int) and receipt['response']['version']==request['expected_version']+1,'committed operation version mismatch')
  require(receipt.get('evidence_id')==request.get('admission_evidence_id'),'committed evidence mismatch')
 def operation(self,key):
  request=self.contract(key)
  # Lookup first, including on resume after an ambiguous successful commit.
  receipt=self.a.lookup(request)
  if receipt is not None:
   self.confirm(key,receipt);return True
  s=self.a.snapshot();attempts=self.s['operation_attempts'].get(key,0)
  if key=='request':
   parent_ready(self.p,s);require(self.clock()<instant(self.p['not_after']),'start intent expired')
  else:
   t=s['profile'].get('tuning') or {}
   if t.get('id')!=self.s['request']['request_id'] or t.get('phase')!='TESTING' or s['profile']['version']!=request['expected_version']:return False
  require(attempts<2,'identical request replay budget exhausted')
  self.s['operation_attempts'][key]=attempts+1;self.save('START_INTENT' if key=='request' else 'CANCEL_INTENT','invoking saved operation')
  try:self.a.execute(request)
  except Uncertain:pass
  receipt=self.a.lookup(request)
  if receipt is None:raise Uncertain('operation outcome unconfirmed; authoritative reconciliation required')
  self.confirm(key,receipt);return True
 def cancel(self,reason):
  self.s['trial_outcome']='rejected';self.save('RESTORING',reason);s=self.a.snapshot();t=s['profile'].get('tuning') or {}
  if t.get('id')!=self.s['request']['request_id']:raise Uncertain('trial ownership changed; no cancellation issued')
  if t.get('phase')=='TESTING' and instant(s['at'])<instant(t['deadline']):
   if 'cancel' not in self.s:
    self.s['cancel']={'request_id':str(uuid.uuid4()),'experiment_id':self.s['request']['experiment_id'],'action':'tune_cancel','expected_version':s['profile']['version'],'tuning_id':t['id']};self.s['cancel_hash']=digest(self.s['cancel']);self.save('CANCEL_INTENT','exact owned cancellation durable')
   try:self.operation('cancel')
   except (Gate,Uncertain):self.save('RESTORING','cancellation not confirmed; observing durable deadline recovery')
 def trial(self):
  self.save('TESTING','awaiting native candidate application');start=self.mono()
  while True:
   s=self.a.snapshot();t=s['profile'].get('tuning') or {}
   require(t.get('id')==self.s['request']['request_id'],'trial ownership changed')
   self.s['deadline']=t['deadline']
   unchanged(self.p,s);require(s['panel']['owner']==self.s['request']['experiment_id'] and s['panel']['config']==self.s['request']['config'] and s['panel']['generation']==self.p['before']['panel']['generation']+1,'candidate desired assignment changed')
   try:active(self.p,s,self.s['request']);break
   except Gate:
    require(t.get('phase')=='TESTING' and instant(s['at'])<instant(t['deadline']) and self.mono()-start<90,'candidate application not proved within budget')
    self.pause(5);self.pulse()
  deadline=instant(self.s['deadline']);self.s['native_candidate']=self.a.native(until=deadline);active(self.p,self.a.snapshot(),self.s['request'])
  self.save('TESTING','measuring one fixed candidate traffic window');self.s['candidate']=self.a.traffic('candidate',until=deadline);active(self.p,self.a.snapshot(),self.s['request'])
  require(accepted(self.s['baseline'],self.s['candidate']),'candidate reliability or latency failed frozen criterion')
  self.s['trial_outcome']='accepted';self.save('RESTORING','candidate accepted; awaiting original deadline without extension')
 def recover(self):
  require('request' in self.s,'no saved trial intent');request=self.contract('request');self.save('RESTORING','reconciling exact durable recovery')
  receipt=self.a.lookup(request)
  if receipt is None:raise Uncertain('no committed receipt; recovery cannot be inferred')
  self.confirm('request',receipt)
  started=self.mono()
  while True:
   s=self.a.snapshot();t=s['profile'].get('tuning') or {}
   require(t.get('id')==request['request_id'],'trial recovery ownership changed')
   deadline=instant(t['deadline']);self.s['deadline']=t['deadline']
   require(instant(s['at'])<=deadline+dt.timedelta(minutes=15) and self.mono()-started<=1200,'restoration observation budget exhausted')
   if t.get('phase')=='RESTORED':
    restored(self.p,s,request);self.s['native_restored']=self.a.native(until=deadline+dt.timedelta(minutes=15));after=self.a.snapshot();restored(self.p,after,request);require(instant(after['at'])<=deadline+dt.timedelta(minutes=15),'restoration proof exceeded budget')
    self.s['restoration_outcome']='verified';self.save('COMPLETE','exact parent restored and native proof passed');return
   require(t.get('phase') in ('TESTING','RESTORING'),'unexpected recovery phase')
   self.pause(5);self.pulse()
 def run(self):
  if self.s['phase'] in TERMINAL:return self.s
  try:
   self.a.stage()
   if self.resuming:
    if 'request' not in self.s:self.save('NO_ACTION','interrupted qualification is never repeated');return self.s
    require(digest(self.s['request'])==self.s['request_hash'],'saved request changed')
    if not self.operation('request'):raise Uncertain('start remains unconfirmed')
    # After interruption, do not replay measurement or declare trial accepted.
    if self.s['trial_outcome']!='accepted':self.cancel('interrupted trial is not performance acceptance')
    self.recover();return self.s
   self.prepare();self.operation('request')
   try:self.trial()
   except (Gate,Uncertain):self.cancel('trial proof incomplete or criterion failed')
   self.recover()
  except (Gate,Uncertain,KeyboardInterrupt) as error:
   reason=str(error) or 'operator interruption'
   if 'request' in self.s:
    self.s['restoration_outcome']='pending';self.save('RECOVERY_PENDING',reason+'; reconciliation required, no replacement trial')
   else:self.save('NO_ACTION',reason+'; all attempted evidence retained')
  return self.s

def status(directory):
 value=json.loads((Path(directory)/'status.json').read_text());value['activity_confirmed']=False
 try:
  current=Path('/proc/'+str(value['pid'])+'/stat').read_text().rsplit(')',1)[1].split()[19]
  boot=Path('/proc/sys/kernel/random/boot_id').read_text().strip()
  value['activity_confirmed']=bool(value['valid_until'] and now()<instant(value['valid_until']) and current==value['pid_start_ticks'] and boot==value['boot_id'])
 except (OSError,KeyError,ValueError):pass
 return value

def main():
 parser=argparse.ArgumentParser(description=__doc__);group=parser.add_mutually_exclusive_group(required=True)
 group.add_argument('--freeze',action='store_true');group.add_argument('--run',action='store_true');group.add_argument('--status',action='store_true')
 parser.add_argument('--directory',required=True);parser.add_argument('--panel');parser.add_argument('--suspects');parser.add_argument('--controls');parser.add_argument('--tools',help='JSON object of reviewed absolute artifact paths for freeze')
 args=parser.parse_args();directory=Path(args.directory).absolute()
 if args.status:print(json.dumps(status(directory),ensure_ascii=False));return 0
 require(os.geteuid()==0,'protected operator account required')
 directory.mkdir(parents=True,mode=0o700,exist_ok=True);st=directory.lstat();require(stat.S_ISDIR(st.st_mode) and st.st_uid==0 and not st.st_mode&0o077,'private nonsymlink run directory required')
 (directory/'tmp').mkdir(mode=0o700,exist_ok=True)
 with lock('/opt/.residential-stability-supervisor.lock'),lock('/opt/.digital-ocean-bot-deploy.lock',shared=True):
  if args.freeze:
   require(not (directory/'policy.json').exists() and not (directory/'state.json').exists(),'job directory already frozen')
   panel=uid(args.panel);suspects=args.suspects.split(',');controls=args.controls.split(',');specs=json.loads(Path(args.tools).read_text());require(set(specs)==TOOLS,'exact tool set required')
   tools={}
   for name,value in specs.items():
    path=Path(value);st=path.lstat();require(path.is_absolute() and stat.S_ISREG(st.st_mode) and st.st_uid==0 and not st.st_mode&0o022,'tool must be a protected regular file');tools[name]={'path':str(path),'sha256':hashlib.sha256(path.read_bytes()).hexdigest()}
   p={'schema':1,'job_id':str(uuid.uuid4()),'panel':panel,'suspects':suspects,'controls':controls,'created_at':now().isoformat(),'not_after':(now()+dt.timedelta(minutes=20)).isoformat(),'tools':tools,'runtime':json.loads(Path('/opt/digital-ocean-bot/build-manifest.json').read_text())}
   a=Adapter(p,directory);p['before']=a.snapshot();validate_policy(p);parent_ready(p,p['before']);a.runtime();atomic(directory/'policy.json',p);print(json.dumps({'job_id':p['job_id'],'frozen':True,'not_after':p['not_after']}));return 0
  require((directory/'policy.json').is_file(),'frozen policy required');p=json.loads((directory/'policy.json').read_text());a=Adapter(p,directory);engine=Engine(p,directory,a)
  try:result=engine.run()
  except Exception:
   # Do not echo subprocess arguments, credentials, SQL errors or arbitrary payloads.
   try:
    engine.s['restoration_outcome']='pending' if 'request' in engine.s else 'not_required';engine.save('RECOVERY_PENDING' if 'request' in engine.s else 'ERROR','local failure; inspect private evidence and reconcile before retry')
   except Exception:pass
   print('supervisor stopped; no success asserted',file=sys.stderr);return 2
  return 0 if result['phase']=='COMPLETE' and result['trial_outcome']=='accepted' else 2

if __name__=='__main__':
 try:raise SystemExit(main())
 except (Gate,OSError,ValueError,KeyError,TypeError):print('precondition or local state invalid; no success asserted',file=sys.stderr);raise SystemExit(2)
