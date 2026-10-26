package configplane

import (
	"os/exec"
	"testing"
)

func TestBatchProofRejectsMixedOrIncompleteResponses(t *testing.T) {
	good := []batchResult{{ID: "a", Verified: true, Classes: map[string]bool{"DIRECT": true, "RESIDENTIAL": false}}, {ID: "b", Reason: "users"}}
	got, e := validateBatchResults([]string{"a", "b"}, good)
	if e != nil || !got["a"].Classes["DIRECT"] || got["b"].Verified {
		t.Fatal("isolated proofs", e)
	}
	for _, bad := range [][]batchResult{good[:1], append(good, good[0]), {good[0], good[0]}, {{ID: "a"}, {ID: "other"}}, {{ID: "a", Classes: map[string]bool{"UNKNOWN": true}}, good[1]}} {
		if _, e := validateBatchResults([]string{"a", "b"}, bad); e == nil {
			t.Fatal("mixed/incomplete result accepted", bad)
		}
	}
}

// Executes the embedded program's actual process/socket/user checks. One bad
// membership must neither succeed nor prevent a valid sibling from completing.
func TestRuntimeBatchExecutesChecksAndIsolatesFailures(t *testing.T) {
	script := `
import ast,pathlib,json,hashlib,types,io
source=pathlib.Path('runtime_users.py').read_text()+'\n'+pathlib.Path('runtime_batch.py').read_text();tree=ast.parse(source)
ns={};exec(compile(ast.Module(body=[n for n in tree.body if isinstance(n,(ast.Import,ast.ImportFrom,ast.FunctionDef))],type_ignores=[]),'runtime_batch.py','exec'),ns)
core='/usr/local/x-ui/bin/xray-linux-amd64'
class P:
 def __init__(self,p):self.p=str(p)
 def __str__(self):return self.p
 def __truediv__(self,v):return P(self.p+'/'+v)
 @property
 def name(self):return self.p.split('/')[-1]
 @property
 def parent(self):return P(self.p.rsplit('/',1)[0])
 def resolve(self):return self
 def is_absolute(self):return self.p.startswith('/')
 def __eq__(self,o):return isinstance(o,P) and self.p==o.p
 def open(self,mode):return io.BytesIO(b'approved')
 def read_bytes(self):
  if self.p.endswith('cmdline'):return b'xray\0-c\0/usr/local/x-ui/bin/config.json\0'
  return b'approved'
 def read_text(self):
  if self.p.endswith('config.json'):return json.dumps({'api':{'tag':'api'},'inbounds':[{'tag':'api','listen':'127.0.0.1','port':5001}]})
  return 'header\n0 00000000:4433 0 0A 0 0 0 0 0 123\n'
 def iterdir(self):return [P('/proc/42')] if self.p=='/proc' else [P(self.p+'/7')]
ns['pathlib']=types.SimpleNamespace(Path=P)
ns['platform']=types.SimpleNamespace(machine=lambda:'x86_64')
ns['os']=types.SimpleNamespace(readlink=lambda p: 'socket:[123]' if '/fd/' in str(p) else core,fsdecode=lambda b:b.decode())
ns['subprocess']=types.SimpleNamespace(run=lambda *a,**k:types.SimpleNamespace(returncode=0,stdout=json.dumps({'users':[{'email':'owned'}]})))
w={'profile_id':'good','port':17459,'core_hashes':[hashlib.sha256(b'approved').hexdigest()],'tag':'owned','emails':['owned'],'probes':[],'probe_classes':[]}
bad=dict(w,profile_id='bad',emails=['missing'])
from concurrent.futures import ThreadPoolExecutor
observation=ns['observe_runtime']()
ns['observe_runtime']=lambda: (_ for _ in ()).throw(AssertionError('runtime reread within batch'))
with ThreadPoolExecutor(max_workers=2) as pool:out=list(pool.map(lambda x:ns['run_job'](x,observation),[bad,w]))
assert out[0]['reason']=='users' and not out[0]['verified'],out
assert out[1]['verified'] and out[1]['reason']=='',out
assert not ns['run_job'](dict(w,core_hashes=['unknown']),observation)['verified']
`
	if out, e := exec.Command("python3", "-c", script).CombinedOutput(); e != nil {
		t.Fatalf("batch runtime: %v %s", e, out)
	}
}
