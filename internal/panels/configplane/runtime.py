# Credentials arrive only over encrypted stdin, never argv, disk or logs.
import hashlib,http.client,json,os,pathlib,socket,ssl,subprocess,sys,time,platform,concurrent.futures
want=json.load(sys.stdin)
class_proofs={}
def sha(v):return hashlib.sha256(v).hexdigest()
def probe_targets(probes,classes):
 # This is the same destination contract as route verification. Residential
 # intentionally denies arbitrary websites, including the Direct probe.
 targets={'DIRECT':('example.com','/'),'RESIDENTIAL':('browserleaks.com','/ip')}
 if not 1<=len(probes)<=2 or len(classes)!=len(probes) or any(c not in targets for c in classes):
  raise ValueError('invalid probe classes')
 return [targets[c] for c in classes]
def check():
 global stage
 stage="binary"
 arch='arm64' if platform.machine() in ('aarch64','arm64') else 'amd64'
 core=pathlib.Path('/usr/local/x-ui/bin/xray-linux-'+arch);conf=core.parent/'config.json'
 config=json.loads(conf.read_text())
 expected_hash=sha(core.read_bytes())
 if expected_hash not in want['core_hashes']:return False
 running=[]
 for p in pathlib.Path('/proc').iterdir():
  if not p.name.isdigit():continue
  try:
   if os.readlink(p/'exe')!=str(core):continue
   args=(p/'cmdline').read_bytes().split(bytes([0]));actual_path=None
   for flag in (b'-c',b'-config'):
    if flag in args:
     actual_path=pathlib.Path(os.fsdecode(args[args.index(flag)+1]))
     if not actual_path.is_absolute():actual_path=(p/'cwd').resolve()/actual_path
   if actual_path is not None and actual_path.resolve()==conf.resolve() and sha((p/'exe').read_bytes())==expected_hash:running.append(p)
  except (OSError,ValueError,IndexError):pass
 if len(running)!=1:return False
 stage="socket"
 proc=running[0];owned=set()
 for fd in (proc/'fd').iterdir():
  try:
   link=os.readlink(fd)
   if link.startswith('socket:['):owned.add(link[8:-1])
  except OSError:pass
 bound=False
 for family in ('tcp','tcp6'):
  for line in (proc/'net'/family).read_text().splitlines()[1:]:
   cols=line.split()
   if cols[3]=='0A' and int(cols[1].split(':')[1],16)==want['port'] and cols[9] in owned:bound=True
 if not bound:return False
 stage="users"
 api=[i for i in config.get('inbounds',[]) if i.get('tag')==config.get('api',{}).get('tag') and i.get('listen') in ('127.0.0.1','::1')]
 if len(api)!=1 or not want.get('tag'):return False
 addr=api[0]['listen'];addr='['+addr+']' if ':' in addr else addr
 result=subprocess.run([str(core),'api','inbounduser','--server='+addr+':'+str(api[0]['port']),'-tag='+want['tag']],capture_output=True,text=True,timeout=5)
 if result.returncode:return False
 users=json.loads(result.stdout).get('users',[])
 if sorted(u.get('email','') for u in users)!=sorted(want['emails']):return False
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
    return probe_profile_https(port,probe_host,probe_path)
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
print(json.dumps({'verified':verified,'reason':'' if verified else stage,'classes':class_proofs},separators=(',',':')))
raise SystemExit(0 if verified else 1)
