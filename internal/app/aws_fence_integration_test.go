package app

import (
	"net/http"
	"strings"
	"testing"
)

func TestAWSTerminationPreservesDisabledAccountCleanupFence(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	id := "eu-west-1:i-12345678"
	execBootstrap(t, db, "UPDATE accounts SET provider='aws',enabled=false,deletion_requested_at=now() WHERE id=$1", f.d.AccountID)
	execBootstrap(t, db, "UPDATE droplets SET provider_resource_id=$2 WHERE account_id=$1", f.d.AccountID, id)
	calls := 0
	transport := providerMutationFence{db: db, account: f.d.AccountID, base: fenceTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})}
	request := func(action, native string) *http.Request {
		r, _ := http.NewRequest("POST", "https://ec2.eu-west-1.amazonaws.com/", strings.NewReader("Action="+action+"&InstanceId.1="+native+"&Version=2016-11-15"))
		return r
	}
	if _, e := transport.RoundTrip(request("RunInstances", "i-12345678")); e == nil || calls != 0 {
		t.Fatal("disabled account created", e, calls)
	}
	if _, e := transport.RoundTrip(request("TerminateInstances", "i-87654321")); e == nil || calls != 0 {
		t.Fatal("foreign resource deletion", e, calls)
	}
	resp, e := transport.RoundTrip(request("TerminateInstances", "i-12345678"))
	if e != nil || calls != 1 {
		t.Fatal("owned cleanup blocked", e, calls)
	}
	resp.Body.Close()
	var fenced bool
	if e = db.QueryRow("SELECT EXISTS(SELECT 1 FROM ip_native_delete_fences WHERE account_id=$1 AND server_id=$2)", f.d.AccountID, id).Scan(&fenced); e != nil || !fenced {
		t.Fatal("IP obligation fence lost", e)
	}
}
func TestOVHDeletionRequiresExactLocalProjectBinding(t *testing.T) {
	db := installerBootstrapDB(t)
	f := newBootstrapFixture(t, db)
	id := "11111111-1111-1111-1111-111111111111"
	project := "0123456789abcdef0123456789abcdef"
	execBootstrap(t, db, "UPDATE accounts SET provider='ovh',external_id=$2,enabled=false,deletion_requested_at=now() WHERE id=$1", f.d.AccountID, "ovh-eu:"+project)
	execBootstrap(t, db, "UPDATE droplets SET provider_resource_id=$2 WHERE account_id=$1", f.d.AccountID, id)
	calls := 0
	tr := providerMutationFence{db: db, account: f.d.AccountID, base: fenceTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})}
	request := func(p, i string) *http.Request {
		r, _ := http.NewRequest("DELETE", "https://eu.api.ovh.com/1.0/cloud/project/"+p+"/instance/"+i, nil)
		return r
	}
	if _, e := tr.RoundTrip(request(project, "22222222-2222-2222-2222-222222222222")); e == nil || calls != 0 {
		t.Fatal("foreign instance deletion", e, calls)
	}
	if _, e := tr.RoundTrip(request(strings.Repeat("a", 32), id)); e == nil || calls != 0 {
		t.Fatal("foreign project deletion", e, calls)
	}
	resp, e := tr.RoundTrip(request(project, id))
	if e != nil || calls != 1 {
		t.Fatal(e, calls)
	}
	resp.Body.Close()
	execBootstrap(t, db, "UPDATE droplets SET state='DELETED' WHERE account_id=$1", f.d.AccountID)
	if _, e := tr.RoundTrip(request(project, id)); e == nil || calls != 1 {
		t.Fatal("stale local binding accepted", e, calls)
	}
}
