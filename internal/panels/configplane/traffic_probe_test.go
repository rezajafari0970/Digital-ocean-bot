package configplane

import (
	"os/exec"
	"strings"
	"testing"
)

func TestTrafficProofRetriesTransientFailuresWithoutWeakeningTLS(t *testing.T) {
	script := `
import pathlib,ssl,types
ns={}
exec(pathlib.Path('traffic_probe.py').read_text(),ns)
def check(sequence,expected,expected_calls,code='ok'):
 calls=[];closed=[];sleeps=[]
 class Connection:
  def __init__(self,host,port,timeout,context):
   assert host=='127.0.0.1' and port==12345 and timeout==8
   assert context.check_hostname and context.verify_mode==ssl.CERT_REQUIRED
   self.index=len(calls);calls.append(self)
  def set_tunnel(self,host,port):assert (host,port)==('browserleaks.com',443)
  def request(self,method,path,headers):
   assert method=='GET' and path=='/ip'
   assert headers=={'Host':'browserleaks.com','Connection':'close'}
  def getresponse(self):
   value=sequence[self.index]
   if isinstance(value,Exception):raise value
   status,body=value
   return types.SimpleNamespace(status=status,read=lambda n:body)
  def close(self):closed.append(self)
 ns['http']=types.SimpleNamespace(client=types.SimpleNamespace(HTTPSConnection=Connection,HTTPException=Exception))
 ns['time']=types.SimpleNamespace(sleep=lambda n:sleeps.append(n))
 detail={}
 result=ns['probe_profile_https'](12345,'browserleaks.com','/ip',detail)
 assert detail=={'code':code,'attempts':expected_calls},detail
 assert result is expected,(sequence,result)
 assert len(calls)==expected_calls and len(closed)==expected_calls,(calls,closed)
 assert len(sleeps)==expected_calls-1 and all(x==0.25 for x in sleeps)
check([(200,b'ok')],True,1)
check([TimeoutError(),(200,b'ok')],True,2)
check([ConnectionResetError(),(200,b'ok')],True,2)
check([(503,b'bad'),(200,b'ok')],True,2)
check([TimeoutError('sensitive'),TimeoutError('sensitive'),(200,b'ok')],False,2,'timeout')
check([(503,b'bad'),(503,b'bad')],False,2,'http_503')
check([ssl.SSLCertVerificationError('sensitive certificate'),(200,b'ok')],False,1,'tls_certificate')
check([(403,b'denied'),(200,b'ok')],False,1,'http_403')
check([(429,b'limited'),(200,b'ok')],False,1,'http_429')
check([(200,b'')],False,1,'empty_body')
check([ConnectionResetError('secret'),ConnectionResetError('secret')],False,2,'transport')
`
	if out, err := exec.Command("python3", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("bounded traffic proof: %v %s", err, out)
	}
	for _, d := range []trafficDiagnostic{{"http_999", 1}, {"http_403\nsecret", 1}, {"secret", 1}, {"timeout", 0}, {"transport", 3}} {
		if d.safe() {
			t.Fatalf("unsafe diagnostic accepted: %#v", d)
		}
	}
	for _, d := range []trafficDiagnostic{{"timeout", 2}, {"http_403", 1}, {"tls_certificate", 1}} {
		if !d.safe() {
			t.Fatalf("safe diagnostic rejected: %#v", d)
		}
	}
	// Both standalone SSH programs must include the same tested helper and use it.
	for _, script := range []string{runtimeScript, runtimeBatchScript} {
		if !strings.HasPrefix(script, trafficProbeScript+"\n") || !strings.Contains(script, "return probe_profile_https(port,probe_host,probe_path,detail)") {
			t.Fatal("runtime does not use the bounded, strict TLS traffic probe")
		}
	}
}
