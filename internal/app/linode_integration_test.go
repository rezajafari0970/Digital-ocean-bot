package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/linode"
	"golang.org/x/crypto/ssh"
	"io"
	"net/http"
	"strings"
	"testing"
)

type linodeTestCredential struct{}

func (linodeTestCredential) Get(context.Context) ([]byte, error) { return []byte("fixture"), nil }
func TestLinodePreservesAccountMutationFence(t *testing.T) {
	db := fenceTestDB(t)
	pub, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	key, e := ssh.NewPublicKey(pub)
	if e != nil {
		t.Fatal(e)
	}
	posts := 0
	transport := providerMutationFence{db: db, account: "test", base: fenceTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"backups_enabled":false,"managed":false}`
		if r.URL.Path == "/v4/profile" {
			body = `{"restricted":false}`
		}
		if r.Method == "POST" {
			posts++
			body = `{"id":7}`
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	opened, e := (linode.Factory{}).Open(context.Background(), providers.OpenRequest{AccountID: "test", HTTPClient: &http.Client{Transport: transport}, Credentials: linodeTestCredential{}})
	if e != nil {
		t.Fatal(e)
	}
	d := opened.(providers.ComputeDriver)
	req := providers.CreateServerRequest{Name: "dob-fixture", RegionID: "us-east", PlanID: "g6-nanode-1", ImageID: "linode/ubuntu24.04", Identity: "fixture-identity", SSHAuthorizedKeys: []string{string(ssh.MarshalAuthorizedKey(key))}}
	if got, e := d.CreateServer(context.Background(), req); e != nil || got.ServerID != "7" || posts != 1 {
		t.Fatal(got, e, posts)
	}
	if _, e = db.Exec("UPDATE accounts SET enabled=false,deletion_requested_at=now() WHERE id='test'"); e != nil {
		t.Fatal(e)
	}
	if _, e = d.CreateServer(context.Background(), req); e == nil || posts != 1 {
		t.Fatal("Linode bypassed mutation fence", e, posts)
	}
}
