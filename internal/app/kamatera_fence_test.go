package app

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/kamatera"
	"net/http"
	"strings"
	"testing"
)

func TestKamateraCleanupRequiresBothJournalAndLocalScope(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	id := "11111111-1111-4111-8111-111111111111"
	execBootstrap(t, db, "UPDATE accounts SET provider='kamatera',external_id=$2,enabled=false,deletion_requested_at=now() WHERE id=$1", f.d.AccountID, kamatera.CredentialScope("fixture-client"))
	execBootstrap(t, db, "UPDATE droplets SET provider_resource_id=$2 WHERE account_id=$1", f.d.AccountID, id)
	calls := 0
	tr := providerMutationFence{db: db, account: f.d.AccountID, base: fenceTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})}
	request := func(path, body, client string) *http.Request {
		r, _ := http.NewRequest("POST", "https://cloudcli.cloudwm.com"+path, strings.NewReader(body))
		r.Header.Set("AuthClientId", client)
		return r
	}
	body := `{"id":"` + id + `","force":true}`
	if _, e := tr.RoundTrip(request("/service/server/terminate", body, "fixture-client")); e == nil {
		t.Fatal("unjournaled deletion")
	}
	execBootstrap(t, db, "INSERT INTO provider_account_resources(account_id,provider,scope,name,resource_id) VALUES($1,'kamatera','server:fixture','fixture',$2)", f.d.AccountID, id)
	for _, r := range []*http.Request{request("/service/server/terminate", body, "wrong-client"), request("/service/server", `{"name":"new"}`, "fixture-client")} {
		if _, e := tr.RoundTrip(r); e == nil {
			t.Fatal("unsafe mutation allowed")
		}
	}
	for _, r := range []*http.Request{request("/service/server/info", `{"name":".*"}`, "fixture-client"), request("/service/server/terminate", body, "fixture-client")} {
		resp, e := tr.RoundTrip(r)
		if e != nil {
			t.Fatal("read or bound cleanup blocked", e)
		}
		resp.Body.Close()
	}
	if calls != 2 {
		t.Fatal("unexpected mutations", calls)
	}
}
