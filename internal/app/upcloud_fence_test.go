package app

import (
	"context"
	"net/http"
	"testing"
)

func TestUpCloudStopAllowedOnlyWithDurableCleanupManifest(t *testing.T) {
	db := fenceTestDB(t)
	for _, q := range []string{
		"ALTER TABLE accounts ADD COLUMN provider text DEFAULT 'upcloud'",
		"UPDATE accounts SET enabled=false,deletion_requested_at=now()",
		"CREATE TABLE provider_cleanup_manifests(account_id text,provider text,server_id text,completed_at timestamptz)",
	} {
		if _, e := db.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	calls := 0
	f := providerMutationFence{db: db, account: "test", base: fenceTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})}
	send := func(method, url string) error {
		r, _ := http.NewRequestWithContext(context.Background(), method, url, nil)
		s, e := f.RoundTrip(r)
		if s != nil {
			s.Body.Close()
		}
		return e
	}
	url := "https://api.upcloud.com/1.3/server/00112233-4455-4677-8899-aabbccddeeff/stop"
	if e := send("POST", url); e == nil {
		t.Fatal("unplanned stop allowed")
	}
	if _, e := db.Exec("INSERT INTO provider_cleanup_manifests VALUES('test','upcloud','00112233-4455-4677-8899-aabbccddeeff',NULL)"); e != nil {
		t.Fatal(e)
	}
	if e := send("POST", url); e != nil {
		t.Fatal("owned cleanup blocked", e)
	}
	for _, u := range []string{"https://api.upcloud.com/1.3/server", "https://api.upcloud.com/1.3/server/another/stop", "http://api.upcloud.com/1.3/server/00112233-4455-4677-8899-aabbccddeeff/stop", "https://other.invalid/1.3/server/00112233-4455-4677-8899-aabbccddeeff/stop"} {
		if e := send("POST", u); e == nil {
			t.Fatal("unrelated mutation allowed", u)
		}
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}
