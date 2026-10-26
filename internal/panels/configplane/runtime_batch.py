# Each job has independent results; bounded concurrency amortizes SSH overhead.
import hashlib,http.client,json,os,pathlib,socket,ssl,subprocess,sys,time,platform,concurrent.futures
def probe_targets(probes,classes):
 # This is the same destination contract as route verification. Residential
 # intentionally denies arbitrary websites, including the Direct probe.
 targets={'DIRECT':('example.com','/'),'RESIDENTIAL':('browserleaks.com','/ip')}
 if not 1<=len(probes)<=2 or len(classes)!=len(probes) or any(c not in targets for c in classes):
  raise ValueError('invalid probe classes')
 return [targets[c] for c in classes]

def file_sha(path):
 h=hashlib.sha256()
 with path.open('rb') as f:
  for block in iter(lambda:f.read(1048576),b''):h.update(block)
 return h.hexdigest()

def observe_runtime():
 # The panel configuration fence is held for this batch. Hash the same runtime
 # once with bounded memory, while every job retains independent user/traffic checks.
 arch='arm64' if platform.machine() in ('aarch64','arm64') else 'amd64'
 core=pathlib.Path('/usr/local/x-ui/bin/xray-linux-'+arch);conf=core.parent/'config.json'
 config=json.loads(conf.read_text());expected_hash=file_sha(core);running=[]
 for p in pathlib.Path('/proc').iterdir():
  if not p.name.isdigit():continue
  try:
   if os.readlink(p/'exe')!=str(core):continue
   args=(p/'cmdline').read_bytes().split(bytes([0]));actual_path=None
   for flag in (b'-c',b'-config'):
    if flag in args:
     actual_path=pathlib.Path(os.fsdecode(args[args.index(flag)+1]))
     if not actual_path.is_absolute():actual_path=(p/'cwd').resolve()/actual_path
   if actual_path is not None and actual_path.resolve()==conf.resolve() and file_sha(p/'exe')==expected_hash:running.append(p)
  except (OSError,ValueError,IndexError):pass
 if len(running)!=1:return None
 proc=running[0];owned=set();ports=set()
 for fd in (proc/'fd').iterdir():
  try:
   link=os.readlink(fd)
   if link.startswith('socket:['):owned.add(link[8:-1])
  except OSError:pass
 for family in ('tcp','tcp6'):
  for line in (proc/'net'/family).read_text().splitlines()[1:]:
   cols=line.split()
   if cols[3]=='0A' and cols[9] in owned:ports.add(int(cols[1].split(':')[1],16))
 return core,config,expected_hash,ports

def run_job(want,observation=None):
 class_proofs={};class_diagnostics={};core_current=None
 def check():
  nonlocal stage,core_current
  stage="binary"
  observed=observation if observation is not None else observe_runtime()
  if observed is None:return False
  core,config,expected_hash,ports=observed
  if want.get('current_core_hashes'):core_current=expected_hash in want['current_core_hashes']
  if expected_hash not in want['core_hashes']:return False
  stage="socket"
  if want['port'] not in ports:return False
  stage="users"
  api=[i for i in config.get('inbounds',[]) if i.get('tag')==config.get('api',{}).get('tag') and i.get('listen') in ('127.0.0.1','::1')]
  if len(api)!=1 or not want.get('tag'):return False
  addr=api[0]['listen'];addr='['+addr+']' if ':' in addr else addr
  result=subprocess.run([str(core),'api','inbounduser','--server='+addr+':'+str(api[0]['port']),'-tag='+want['tag']],capture_output=True,text=True,timeout=5)
  if result.returncode:return False
  users=json.loads(result.stdout).get('users',[])
  if not native_users_match(users,want):return False
  stage="traffic"
  probes=want.get('probes',[])
  classes=want.get('probe_classes',[])
  if not probes and not classes:return True
  targets=probe_targets(probes,classes)
  ports=[];inbounds=[];outbounds=[];rules=[]
  for i,outbound in enumerate(probes):
   with socket.socket() as sock:
    sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
   if port in ports:return False
   ports.append(port);tag='profile-probe-'+str(i);outbound=dict(outbound,tag=tag)
   inbounds.append({'tag':tag,'listen':'127.0.0.1','port':port,'protocol':'http','settings':{}})
   outbounds.append(outbound);rules.append({'type':'field','inboundTag':[tag],'outboundTag':tag})
  client={'log':{'loglevel':'none'},'inbounds':inbounds,'outbounds':outbounds,'routing':{'rules':rules}}
  child=subprocess.Popen([str(core),'run','-config','stdin:'],stdin=subprocess.PIPE,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
  try:
   child.stdin.write(json.dumps(client,separators=(',',':')).encode());child.stdin.close()
   def probe(index):
    port=ports[index]
    try:
     deadline=time.monotonic()+3
     while True:
      if child.poll() is not None:return False
      try:
       with socket.create_connection(('127.0.0.1',port),timeout=.2):pass
       break
      except OSError:
       if time.monotonic()>deadline:return False
       time.sleep(.05)
     probe_host,probe_path=targets[index]
     detail={}
     class_diagnostics[classes[index]]=detail
     return probe_profile_https(port,probe_host,probe_path,detail)
    except Exception:return False
   with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
    results=list(pool.map(probe,range(len(ports))))
   class_proofs.update(dict(zip(classes,results)))
   return any(results)
 
  finally:
   if child.poll() is None:child.terminate()
   try:child.wait(timeout=2)
   except subprocess.TimeoutExpired:child.kill();child.wait(timeout=2)
 stage="initial"
 verified=False
 try:verified=check()
 except Exception:pass
 return {'profile_id':want['profile_id'],'verified':verified,'reason':'' if verified else stage,'classes':class_proofs,'class_diagnostics':class_diagnostics,'core_current':core_current}

jobs=json.load(sys.stdin)
if not isinstance(jobs,list) or len(jobs)>64:raise ValueError('invalid batch')
try:observation=observe_runtime()
except Exception:observation=None
if observation is None:
 results=[{'profile_id':j['profile_id'],'verified':False,'reason':'binary','classes':{}} for j in jobs]
else:
 with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:results=list(pool.map(lambda j:run_job(j,observation),jobs))
print(json.dumps({'results':results},separators=(',',':')))
