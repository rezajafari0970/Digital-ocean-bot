"""Read-only sequential native route proofs through one encrypted SSH input."""
import http.client,json,signal,sys,time,urllib.parse
def main():
 p=json.load(sys.stdin)
 forms=p['forms']
 if not 1<=len(forms)<=256 or not 1<=p['port']<=65535:raise ValueError()
 path=p['path']
 if not path.startswith('/') or any(x in path for x in ('\r','\n','?','#','..')):raise ValueError()
 headers={'Content-Type':'application/x-www-form-urlencoded','X-CSRF-Token':p['csrf'],'Cookie':p['cookie']}
 if any('\r' in v or '\n' in v for v in headers.values()):raise ValueError()
 deadline=min(float(p['deadline']),time.time()+10)
 conn=http.client.HTTPConnection('127.0.0.1',p['port'],timeout=3)
 out=[]
 try:
  for i,form in enumerate(forms):
   if len(form)>4096 or not set(urllib.parse.parse_qs(form))<=set(('port','network','inboundTag','email','protocol','domain','ip')):raise ValueError()
   remaining=deadline-time.time()
   if remaining<=0:raise TimeoutError()
   conn.timeout=min(3,remaining)
   if conn.sock is not None:conn.sock.settimeout(conn.timeout)
   conn.request('POST',path+'panel/api/xray/routeTest',form,headers)
   response=conn.getresponse()
   raw=response.read(16385)
   if len(raw)>16384:raise ValueError()
   v=json.loads(raw);obj=v.get('obj') or {}
   tag=obj.get('outboundTag','')
   if not isinstance(tag,str) or len(tag)>256:raise ValueError()
   item={'index':i,'status':response.status,'success':v.get('success') is True,
         'matched':obj.get('matched') is True,'tag':tag,
         'starting':'rpc error: code = Unavailable' in str(v.get('msg',''))}
   out.append(item)
   if not item['success']:break
   # Native Sanaei closes its shared gRPC client per call: never parallelize a panel.
   if i+1<len(forms):time.sleep(.005)
 finally:conn.close()
 print(json.dumps({'results':out},separators=(',',':')))
if __name__=='__main__':
 try:main()
 except Exception:print('{"error":"native_route_batch_unavailable"}');sys.exit(1)
