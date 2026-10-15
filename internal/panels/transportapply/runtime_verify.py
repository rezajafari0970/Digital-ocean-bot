
def sha(v):return hashlib.sha256(v).hexdigest()
def check():
 core=pathlib.Path('/usr/local/x-ui/bin/xray-linux-amd64');conf=core.parent/'config.json'
 config=json.loads(conf.read_text())
 matches=[i for i in config.get('inbounds',[]) if i.get('protocol')=='vless' and str(i.get('port'))==str(want['port'])]
 if len(matches)!=1:raise ValueError("listener_count")
 i=matches[0];s=i.get('streamSettings',{});v=i.get('settings',{})
 if s.get('network')!=want['network'] or s.get('security')!=want['security']:raise ValueError("network_security")
 if v.get('flow','')!=want.get('flow','') or sha(v.get('decryption','none').encode())!=want['decryption_hash']:raise ValueError("flow_encryption")
 r=s.get('realitySettings',{})
 if sha(r.get('privateKey','').encode())!=want['private_hash'] or r.get('shortIds')!=want['shortIds'] or r.get('serverNames')!=want['serverNames']:raise ValueError("reality_identity")
 if s.get('finalmask')!=want.get('finalmask'):raise ValueError("finalmask")
 if want['network']=='xhttp' and s.get('xhttpSettings')!=want.get('xhttp'):raise ValueError("xhttp_settings")
 actual_clients=sorted(sha((str(c.get('id',''))+chr(0)+str(c.get('email',''))).encode()) for c in v.get('clients',[]))
 if actual_clients!=want['clients']:raise ValueError('client_membership')
 for c in v.get('clients',[]):
  if c.get('flow','') not in ('',want.get('flow','')):raise ValueError("client_flow")
 expected_hash=sha(core.read_bytes());running=[]
 boot=pathlib.Path("/proc/sys/kernel/random/boot_id").read_text().strip()
 for p in pathlib.Path('/proc').iterdir():
  if not p.name.isdigit():continue
  try:
   if os.readlink(p/'exe')!=str(core):continue
   args=(p/'cmdline').read_bytes().split(bytes([0]));actual=None
   for flag in (b'-c',b'-config'):
    if flag in args:
     actual=pathlib.Path(os.fsdecode(args[args.index(flag)+1]))
     if not actual.is_absolute():actual=(p/'cwd').resolve()/actual
   if actual is not None and actual.resolve()==conf.resolve() and sha((p/'exe').read_bytes())==expected_hash:
    ticks=(p/'stat').read_text().rsplit(') ',1)[1].split()[19]
    running.append((p,p.name+':'+ticks+':'+boot))
  except (OSError,ValueError,IndexError):pass
 if len(running)!=1:raise ValueError("running_executable")
 proc,identity=running[0]
 if not want.get('previous_process') or identity==want['previous_process']:raise ValueError('process_not_replaced')
 # Prove that this exact process owns the active listening socket.
 owned=set()
 for fd in (proc/'fd').iterdir():
  try:
   link=os.readlink(fd)
   if link.startswith('socket:['):owned.add(link[8:-1])
  except OSError:pass
 bound=False
 for family in ('tcp','tcp6'):
  for line in (proc/'net'/family).read_text().splitlines()[1:]:
   cols=line.split()
   if cols[3]=='0A' and int(cols[1].split(':')[1],16)==int(want['port']) and cols[9] in owned:bound=True
 if not bound:raise ValueError('active_listener_missing')
 # Query the running core, not its startup file, for actual authenticated users.
 api_tag=config.get('api',{}).get('tag')
 api=[x for x in config.get('inbounds',[]) if x.get('tag')==api_tag and x.get('listen') in ('127.0.0.1','::1')]
 if len(api)!=1:raise ValueError('core_api_unavailable')
 address=('['+api[0]['listen']+']') if ':' in api[0]['listen'] else api[0]['listen']
 response=subprocess.run([str(core),'api','inbounduser','--server='+address+':'+str(api[0]['port']),'-tag='+i['tag']],capture_output=True,text=True,timeout=8)
 if response.returncode:raise ValueError('active_users_unavailable')
 users=json.loads(response.stdout).get('users',[])
 active=sorted(sha((u.get('account',{}).get('id','')+chr(0)+u.get('email','')).encode()) for u in users)
 if active!=want['clients']:raise ValueError('active_users_mismatch')
 if any(u.get('account',{}).get('flow','')!=want.get('flow','') for u in users):raise ValueError('active_flow_mismatch')
 env=dict(os.environ,XRAY_LOCATION_ASSET=str(core.parent))
 return subprocess.run([str(core),'run','-test','-config',str(conf)],env=env,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=15).returncode==0
ok=False
reason="unknown"
for attempt in range(4):
 try:
  if check():ok=True;break
 except Exception as e:reason=str(e) if isinstance(e,ValueError) else "runtime_exception"
 time.sleep(1)
print(json.dumps({"verified":ok,"reason":"" if ok else reason},separators=(',',':')))
raise SystemExit(0 if ok else 1)
