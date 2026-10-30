package adminapi
import(
 "context"
 "net"
 "net/http/httptest"
 "strings"
 "testing"
 "time"
 "github.com/rezajafari0970/Digital-ocean-bot/internal/app"
 "github.com/rezajafari0970/Digital-ocean-bot/internal/auth"
 "github.com/rezajafari0970/Digital-ocean-bot/internal/providerbrowser"
 "github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
 "github.com/rezajafari0970/Digital-ocean-bot/internal/providers/vultr"
)
func TestProviderBrowserOwnershipAndNoDirect(t *testing.T){
 ctx,cancel:=context.WithCancel(context.Background());defer cancel();a,b:=net.Pipe();defer b.Close()
 peer:=providerbrowser.NewPeer(ctx,a);peer.Start();defer peer.Close()
 session:=&providerBrowserSession{owner:"owner",login:"login",peer:peer,cancel:cancel,expires:time.Now().Add(time.Minute)}
 s:=&Server{ProviderBrowsers:map[string]*providerBrowserSession{"opaque":session}}
 for _,p:=range []auth.Principal{{UserID:"other",SessionID:"login",Role:auth.Admin},{UserID:"owner",SessionID:"other-login",Role:auth.Admin}}{
  r:=httptest.NewRequest("POST","/",nil);r.SetPathValue("id","opaque");r=r.WithContext(context.WithValue(r.Context(),principalKey{},p));w:=httptest.NewRecorder()
  if s.browserSession(w,r)!=nil||w.Code!=404{t.Fatal("another owner/login could read browser",w.Code)}
  w=httptest.NewRecorder();s.stopProviderBrowser(w,r);if len(s.ProviderBrowsers)!=1{t.Fatal("another login closed browser")}
 }
 s.closeProviderBrowsers("login");select{case <-peer.Done():default:t.Fatal("logout did not close tunnels")}
 registry:=providers.NewRegistry();registry.Register(vultr.Factory{});s=&Server{Container:app.Container{Providers:registry}}
 for _,body:=range []string{`{"provider":"vultr","width":390,"height":800}`,`{"provider":"vultr","proxy_country":"US","width":390,"height":800}`,`{"provider":"vultr","url":"https://unregistered.invalid/","width":390,"height":800}`} {
  r:=httptest.NewRequest("POST","/",strings.NewReader(body));r=r.WithContext(context.WithValue(r.Context(),principalKey{},auth.Principal{UserID:"owner",SessionID:"login",Role:auth.Admin}));w:=httptest.NewRecorder();s.startProviderBrowser(w,r)
  if w.Code!=400{t.Fatal("unproxied or arbitrary entrypoint accepted",w.Code,w.Body.String())}
  if w.Header().Get("Cache-Control")!="no-store"{t.Fatal("browser control response cacheable")}
 }
}
