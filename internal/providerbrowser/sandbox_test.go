package providerbrowser

import (
 "context"
 "crypto/sha256"
 "encoding/base64"
 "errors"
 "fmt"
 "net"
 "net/http"
 "net/http/httptest"
 "os"
 "os/exec"
 "strings"
 "sync/atomic"
 "testing"
 "time"
)

// Executed only by the disposable CI socket-activated unit, with the production
// Worker and sandbox directives. The wrapper trusts only the fixture's SPKI.
func TestProviderBrowserWorkerProcess(t *testing.T){
 if os.Getenv("DOB_TEST_BROWSER_WORKER")!="1"{return}
 fail:=func(e error){fmt.Fprintln(os.Stderr,e);os.Exit(2)}
 if os.Getuid()==0{fail(errors.New("worker is root"))}
 if e:=VerifyIsolation();e!=nil{fail(e)}
 for _,path:=range []string{"/etc/dob-browser-host-fixture","/var/lib/dob-browser-host-fixture","/run/dob-browser-host-fixture"}{if _,e:=os.Stat(path);!os.IsNotExist(e){fail(errors.New("host filesystem visible"))}}
 conn,e:=net.FileConn(os.Stdin);if e!=nil{fail(e)}
 ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute);defer cancel()
 p:=NewPeer(ctx,conn);w:=NewWorker(p,"/usr/local/lib/datacenter-bot/browser-fixture-chrome");p.Handle=w.Handle;p.Start();<-p.Done();p.Wait();w.Close();os.Exit(0)
}
func TestProviderBrowserNativeSandbox(t *testing.T){
 if os.Getenv("DOB_RUN_PROVIDER_BROWSER_SANDBOX")!="1"{t.Skip("disposable systemd sandbox opt-in")}
 var direct,requests atomic.Int64
 trap:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){direct.Add(1);w.WriteHeader(200)}));defer trap.Close()
 server:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  requests.Add(1);w.Header().Set("Content-Type","text/html")
  fmt.Fprintf(w,`<!doctype html><html><body><input id="value"><button onclick="document.title=document.querySelector('input').value">Set title</button><script>
const q=new URLSearchParams(location.search).get('set');if(q){document.cookie='identity='+q+';SameSite=Lax';localStorage.identity=q}
document.title=(document.cookie||'empty')+'|'+(localStorage.identity||'empty');
fetch(%q).catch(()=>{});try{const p=new RTCPeerConnection({iceServers:[{urls:'stun:1.1.1.1:3478'}]});p.createDataChannel('x');p.createOffer().then(x=>p.setLocalDescription(x));setTimeout(()=>p.close(),1000)}catch{}
</script></body></html>`,trap.URL)
 }));defer server.Close()
 hash:=sha256.Sum256(server.Certificate().RawSubjectPublicKeyInfo)
 if b,e:=exec.Command("sudo","systemctl","set-environment","DOB_TEST_BROWSER_SPKI="+base64.StdEncoding.EncodeToString(hash[:])).CombinedOutput();e!=nil{t.Fatal(string(b),e)}
 defer exec.Command("sudo","systemctl","unset-environment","DOB_TEST_BROWSER_SPKI").Run()
 ctx,cancel:=context.WithTimeout(context.Background(),90*time.Second);defer cancel()
 var blocked atomic.Int64
 open:=func(raw string)(*Peer,State){
  conn,e:=net.Dial("unix","/run/datacenter-provider-browser.sock");if e!=nil{t.Fatal(e)}
  p:=NewPeer(ctx,conn);p.Dial=func(ctx context.Context,addr string)(net.Conn,error){if addr!="fixture.invalid:443"{blocked.Add(1);return nil,errors.New("destination blocked")};var d net.Dialer;return d.DialContext(ctx,"tcp",server.Listener.Addr().String())};p.Start();t.Cleanup(p.Close)
  s,e:=p.Call(ctx,Action{Kind:"start",URL:raw,Timezone:"Europe/Berlin",Width:390,Height:800});if e!=nil{t.Fatal("sandbox start",e)}
  return p,s
 }
 first,_:=open("https://fixture.invalid/?set=first")
 second,_:=open("https://fixture.invalid/")
 frame:=func(p *Peer,want string)State{deadline:=time.Now().Add(15*time.Second);for time.Now().Before(deadline){s,e:=p.Call(ctx,Action{Kind:"frame"});if e!=nil{t.Fatal(e)};if s.Title==want{if !s.Isolated||len(s.Image)==0||s.Width!=390{t.Fatal("invalid frame",s.Isolated,s.Width)};return s};time.Sleep(100*time.Millisecond)};t.Fatal("fixture did not render",want);return State{}}
 frame(first,"identity=first|first");frame(second,"empty|empty")
 if requests.Load()<2{t.Fatal("browser did not reach fixture through bridge")}
 if direct.Load()!=0{t.Fatal("browser contacted host loopback directly")}
 first.Close();select{case <-first.Done():case <-time.After(time.Second):t.Fatal("closed browser remained alive")}
 if _,e:=first.Call(ctx,Action{Kind:"frame"});e==nil{t.Fatal("closed session served pixels")}
 frame(second,"empty|empty")
 // Unsupported executable input must never be forwarded as an eval command.
 if _,e:=second.Call(ctx,Action{Kind:"eval",Text:"fetch('http://169.254.169.254/')"});e==nil{t.Fatal("arbitrary CDP/eval accepted")}
 if _,e:=second.Call(ctx,Action{Kind:"navigate",URL:"file:///etc/passwd"});e==nil{t.Fatal("local file navigation accepted")}
 if strings.Contains(frame(second,"empty|empty").URL,"first"){t.Fatal("cross-session navigation")}
 t.Logf("native isolated browsers: requests=%d, direct=%d, rejected destinations=%d; cookies/storage separate",requests.Load(),direct.Load(),blocked.Load())
}
