#!/usr/bin/env python3
"""Measure an owned WS+TLS config on the actual operator network using official Xray."""
import argparse,base64,concurrent.futures,datetime,http.client,ipaddress,json,pathlib,socket,ssl,subprocess,time,urllib.parse,urllib.request,uuid

def outbound(uri,address):
 if uri.startswith('vmess://'):
  b=uri[8:];v=json.loads(base64.b64decode(b+'='*((-len(b))%4)));protocol='vmess';host=v.get('sni') or v.get('host');port=int(v['port']);path=v.get('path','/');user={'id':v['id'],'security':'auto'}
  if v.get('net')!='ws' or v.get('tls')!='tls':raise ValueError('WS+TLS required')
 else:
  u=urllib.parse.urlsplit(uri);q=urllib.parse.parse_qs(u.query);protocol=u.scheme;host=q.get('sni',[''])[0];port=u.port;path=q.get('path',['/'])[0]
  if protocol not in ('vless','trojan') or q.get('security')!=['tls'] or q.get('type')!=['ws']:raise ValueError('WS+TLS required')
  secret=urllib.parse.unquote(u.username or '');user={'id':secret,'encryption':'none'} if protocol=='vless' else {'password':secret}
 if not host or not port:raise ValueError('Owned hostname and port required')
 endpoint={'address':str(ipaddress.ip_address(address)),'port':port}
 if protocol=='trojan':endpoint.update(user);settings={'servers':[endpoint]}
 else:endpoint['users']=[user];settings={'vnext':[endpoint]}
 return {'protocol':protocol,'settings':settings,'streamSettings':{'network':'ws','security':'tls','tlsSettings':{'serverName':host,'allowInsecure':False},'wsSettings':{'path':path,'headers':{'Host':host}}}}

def probe(core,uri,ip):
 out=outbound(uri,ip)
 with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
 config={'log':{'loglevel':'none'},'inbounds':[{'listen':'127.0.0.1','port':port,'protocol':'http','settings':{}}],'outbounds':[out]}
 child=subprocess.Popen([core,'run','-config','stdin:'],stdin=subprocess.PIPE,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 try:
  child.stdin.write(json.dumps(config).encode());child.stdin.close();deadline=time.monotonic()+3
  while time.monotonic()<deadline:
   if child.poll() is not None:raise RuntimeError('Core rejected probe')
   try:
    with socket.create_connection(('127.0.0.1',port),timeout=.2):break
   except OSError:time.sleep(.05)
  start=time.monotonic();conn=http.client.HTTPSConnection('127.0.0.1',port,timeout=12,context=ssl.create_default_context());conn.set_tunnel('example.com',443)
  try:
   conn.request('GET','/',headers={'Host':'example.com','Connection':'close'});response=conn.getresponse();response.read(1024)
   if response.status not in (200,301,302):raise RuntimeError('HTTP verification failed')
  finally:conn.close()
  return {'ip':ip,'latency_ms':max(1,round((time.monotonic()-start)*1000)),'protocol_verified':True}
 finally:
  if child.poll() is None:child.terminate()
  try:child.wait(timeout=2)
  except subprocess.TimeoutExpired:child.kill();child.wait(timeout=2)

def main():
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('--target-file')
 for arg in ['api-url','token-file','uri-file','record-id']:p.add_argument('--'+arg)
 p.add_argument('--operator',required=True,choices=['mci','irancell','rightel','mokhaberat','shatel','other']);p.add_argument('--xray',default='xray');p.add_argument('--ip',action='append',default=[]);p.add_argument('--submit',action='store_true');a=p.parse_args()
 def api(path,data=None):
  u=urllib.parse.urlsplit(a.api_url or '')
  if u.scheme!='https' or u.username or u.password or u.query or u.fragment:raise ValueError('HTTPS controller URL required')
  token_path=pathlib.Path(a.token_file)
  if token_path.stat().st_mode&0o077:raise ValueError('Token file must have mode 600')
  token=token_path.read_text().strip()
  request=urllib.request.Request(a.api_url.rstrip('/')+path,data=json.dumps(data).encode() if data is not None else None,headers={'Authorization':'Bearer '+token,'Content-Type':'application/json'})
  with urllib.request.urlopen(request,timeout=20) as response:return json.load(response)
 if a.target_file:
  target=json.loads(pathlib.Path(a.target_file).read_text());uri=target['uri'];a.record_id=target['record_id'];ranges=[ipaddress.ip_network(x) for x in target['ranges']]
 else:
  if not a.uri_file or not a.record_id:raise ValueError('Use --target-file or --uri-file and --record-id')
  uri=pathlib.Path(a.uri_file).read_text().strip();catalog=api('/api/v1/configs/cloudflare/ips');ranges=[ipaddress.ip_network(x['network']) for x in catalog['ranges']]
 if not ranges:raise ValueError('Refresh official ranges first')
 ips=a.ip or [str(n.network_address+1) for n in ranges if n.version==4][:32]
 if len(ips)>64 or any(not any(ipaddress.ip_address(ip) in n for n in ranges) for ip in ips):raise ValueError('IP outside official ranges')
 def run(ip):
  try:return probe(a.xray,uri,ip)
  except Exception:return {'ip':ip,'protocol_verified':False}
 with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:results=list(pool.map(run,ips))
 for result in results:
  if result['protocol_verified']:
   result.update(id=str(uuid.uuid4()),record_id=str(uuid.UUID(a.record_id)),operator=a.operator,measured_at=datetime.datetime.now(datetime.timezone.utc).isoformat())
   if a.submit:api('/api/v1/configs/cloudflare/measurements',result)
 print(json.dumps({'operator':a.operator,'source':'operator_probe','submitted':a.submit,'results':results},indent=2))
if __name__=='__main__':
 try:main()
 except Exception as exc:
  print(json.dumps({'error':'probe_failed','type':type(exc).__name__}));raise SystemExit(1)
