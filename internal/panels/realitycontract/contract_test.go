package realitycontract

import (
	"encoding/json"
	"net/url"
	"testing"
)

func TestPayloadAndURIStaySymmetric(t *testing.T) {
	m := Managed{Remark: "canary", Host: "167.172.28.228", Port: 18443, UUID: "eb44dbca-6064-4085-8145-ca022e5b2f65", Email: "managed", Target: "www.cloudflare.com", TargetPort: 443, SNI: "www.cloudflare.com", ServerNames: []string{"www.cloudflare.com", "cloudflare.com"}, PrivateKey: "private", PublicKey: "public", ShortID: "04a54a16"}
	p, e := m.Payload()
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(p)
	var x map[string]any
	if json.Unmarshal(raw, &x) != nil {
		t.Fatal("payload json")
	}
	settings := x["settings"].(map[string]any)
	if settings["decryption"] != "none" || settings["encryption"] != "none" {
		t.Fatalf("vless crypto contract=%v/%v", settings["decryption"], settings["encryption"])
	}
	client := settings["clients"].([]any)[0].(map[string]any)
	if client["flow"] != "xtls-rprx-vision" {
		t.Fatalf("server flow=%v", client["flow"])
	}
	stream := x["streamSettings"].(map[string]any)
	rs := stream["realitySettings"].(map[string]any)
	if rs["dest"] != "www.cloudflare.com:443" {
		t.Fatalf("dest=%v", rs["dest"])
	}
	u, e := m.URI()
	if e != nil {
		t.Fatal(e)
	}
	parsed, e := url.Parse(u)
	if e != nil {
		t.Fatal(e)
	}
	q := parsed.Query()
	if q.Get("flow") != "xtls-rprx-vision" || q.Get("sni") != "www.cloudflare.com" || q.Get("sid") != "04a54a16" {
		t.Fatalf("client mismatch %v", q)
	}
	if parsed.Port() != "18443" {
		t.Fatalf("port=%s", parsed.Port())
	}
}
