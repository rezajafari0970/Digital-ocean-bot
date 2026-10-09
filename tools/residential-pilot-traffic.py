#!/usr/bin/env python3
"""Fixed private traffic collector for the bounded stability supervisor."""
from pathlib import Path
import os,sys,json,subprocess,tempfile,socket,time,urllib.parse,datetime,hashlib,re,concurrent.futures,signal,ctypes
panel,label,directory,binary=sys.argv[1:5];b=Path(directory);assert b.is_absolute() and Path(binary).is_absolute();assert re.fullmatch(r'[a-f0-9-]{36}',panel) and re.fullmatch(r'[a-z0-9-]+',label)
env=os.environ.copy();env['PGOPTIONS']='-c default_transaction_read_only=on -c statement_timeout=10000'
sql="SELECT COALESCE(jsonb_agg(t),'[]'::jsonb) FROM (SELECT DISTINCT ON(c.route_class) c.route_class,o.uri FROM output_config_snapshots o JOIN panel_client_routes c ON c.panel_id=o.panel_id AND split_part(split_part(o.uri,'@',1),'://',2)=c.client_id WHERE o.panel_id='"+panel+"' AND o.visible_until>clock_timestamp()+interval '180 seconds' AND o.last_seen_at>clock_timestamp()-interval '15 seconds' AND c.route_class IN('DIRECT','RESIDENTIAL') AND c.effective_class=c.route_class ORDER BY c.route_class,o.last_seen_at DESC,o.uri)t"
p=subprocess.run(['psql',env['DATABASE_URL'],'-XAt','-v','ON_ERROR_STOP=1','-c',sql],capture_output=True,text=True,env=env,timeout=12)
if p.returncode:raise SystemExit('fresh client read unavailable')
rows=json.loads(p.stdout)
if {x['route_class'] for x in rows}!={'DIRECT','RESIDENTIAL'}:raise SystemExit('fresh paired classes unavailable')
def freeport():
 s=socket.socket();s.bind(('127.0.0.1',0));p=s.getsockname()[1];s.close();return p
inbounds=[];outbounds=[{'tag':'blocked','protocol':'blackhole'}];rules=[];ports={}
for row in rows:
 cls=row['route_class'];u=urllib.parse.urlsplit(row['uri']);q=urllib.parse.parse_qs(u.query)
 def one(k,d=''):return q.get(k,[d])[0]
 assert u.scheme=='vless' and one('security')=='reality' and one('type','tcp')=='tcp'
 ports[cls]=freeport();inbounds.append({'tag':cls,'listen':'127.0.0.1','port':ports[cls],'protocol':'socks','settings':{'auth':'noauth'}})
 outbounds.append({'tag':'tunnel-'+cls,'protocol':'vless','settings':{'vnext':[{'address':u.hostname,'port':u.port,'users':[{'id':u.username,'encryption':'none','flow':one('flow')}]}]},'streamSettings':{'network':'tcp','security':'reality','realitySettings':{'serverName':one('sni'),'fingerprint':one('fp','chrome'),'publicKey':one('pbk'),'shortId':one('sid'),'spiderX':one('spx','/')}}})
 rules.append({'type':'field','inboundTag':[cls],'outboundTag':'tunnel-'+cls})
cases=[]
for roundno in range(3):
 for name,url in [('ads','https://pagead2.googlesyndication.com/pagead/js/adsbygoogle.js'),('gpt','https://securepubads.g.doubleclick.net/tag/js/gpt.js'),('browserleaks','https://browserleaks.com/ip')]:cases.append((name+'-'+str(roundno),'RESIDENTIAL',url,200,False))
for host in ['www.gstatic.com','connectivitycheck.gstatic.com','www.google.com']:cases.append(('probe-'+host,'RESIDENTIAL','https://'+host+'/generate_204',204,False))
for url in ['https://example.com/','https://1.1.1.1/','https://dns.google/dns-query']:cases.append(('deny-'+urllib.parse.urlsplit(url).hostname,'RESIDENTIAL',url,0,False))
cases += [('direct-positive','DIRECT','https://example.com/',200,False),('egress-res','RESIDENTIAL','https://browserleaks.com/ip',200,True),('egress-direct','DIRECT','https://browserleaks.com/ip',200,True)]
with tempfile.TemporaryDirectory(prefix='pilot-',dir=b/'tmp') as td:
 f=Path(td)/'client.json';f.write_text(json.dumps({'log':{'loglevel':'none'},'inbounds':inbounds,'outbounds':outbounds,'routing':{'rules':rules}}));f.chmod(0o600)
 proc=subprocess.Popen([binary,'run','-config',str(f)],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,preexec_fn=lambda:ctypes.CDLL(None).prctl(1,signal.SIGKILL))
 def cleanup(*unused):
  proc.terminate()
  try:proc.wait(timeout=3)
  except subprocess.TimeoutExpired:proc.kill();proc.wait()
 signal.signal(signal.SIGTERM,lambda *a:(cleanup(),sys.exit(2)))
 try:
  for _ in range(80):
   if proc.poll() is not None:raise SystemExit('pilot client startup failed')
   try:
    with socket.create_connection(('127.0.0.1',ports['RESIDENTIAL']),timeout=.1):break
   except OSError:time.sleep(.05)
  def attempt(case):
   name,cls,url,want,body=case;dest=Path(td)/(name+'.body');args=['curl','--disable','--retry','0','--silent','--noproxy','','--socks5-hostname','127.0.0.1:'+str(ports[cls]),'--connect-timeout','4','--max-time','6','--output',str(dest),'--write-out','%{json}']
   if not body:args+=['--head']
   started=datetime.datetime.now(datetime.timezone.utc).isoformat();p=subprocess.run(args+[url],capture_output=True,text=True,timeout=8)
   try:v=json.loads(p.stdout)
   except Exception:v={}
   status=v.get('http_code',0);passed=(p.returncode==0 and status==want) if want else (p.returncode!=0 and status==0);egress=None
   if body and passed:
    m=re.search(r'id="client-ipv4"[^>]*data-ip="([^"]+)"',dest.read_text(errors='replace'));egress=hashlib.sha256(m.group(1).encode()).hexdigest() if m else None;passed=bool(egress)
   return {'case':name,'class':cls,'at':started,'passed':passed,'http':status,'curl_code':p.returncode,'seconds':v.get('time_total'),'egress_sha256':egress}
  with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:results=list(pool.map(attempt,cases))
  alive=proc.poll() is None
 finally:cleanup()
e={x['case']:x['egress_sha256'] for x in results if x['egress_sha256']};distinct=len(e)==2 and e['egress-res']!=e['egress-direct']
report={'at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'panel':panel,'phase':label,'cases':results,'egress_distinct':distinct,'local_core_healthy':alive,'scope':'fresh actual RES/DIRECT VLESS; static HEADs and BrowserLeaks egress; not mobile latency or ad show-rate','passed':all(x['passed'] for x in results) and distinct and alive}
(b/(label+'.json')).write_text(json.dumps(report,indent=2)+'\n');print(json.dumps({'phase':label,'passed':report['passed'],'cases':len(results),'failures':[x['case'] for x in results if not x['passed']],'egress_distinct':distinct}));sys.exit(0 if report['passed'] else 1)
