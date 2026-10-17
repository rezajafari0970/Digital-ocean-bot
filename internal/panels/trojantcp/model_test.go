package trojantcp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func TestTrojanPortIsolationAndNativeOwnership(t *testing.T) {
	for _, p := range []int{0, 22, 80, 443, 1023, 18080, 65536} {
		if ValidatePort(p, 2053, nil, nil) == nil {
			t.Fatalf("reserved port accepted: %d", p)
		}
	}
	if ValidatePort(8443, 8443, nil, nil) == nil || ValidatePort(8443, 2053, []int{443, 8443}, nil) == nil || ValidatePort(8443, 2053, nil, []int{8443}) == nil {
		t.Fatal("collision accepted")
	}
	c, e := NewCredentials("fixture")
	if e != nil {
		t.Fatal(e)
	}
	if c.Direct.ID == c.Residential.ID || c.Direct.Password == c.Residential.Password {
		t.Fatal("identity reused")
	}
	p := Policy{Port: 8443, Direct: true, Residential: true}
	payload, e := Build(p, "192.0.2.1", c.Clients(p))
	if e != nil {
		t.Fatal(e)
	}
	payload["id"] = int64(7)
	b, _ := json.Marshal(payload)
	reality := json.RawMessage(`{"id":1,"remark":"dob:global-reality:00443","protocol":"vless","port":443,"enable":true,"settings":{"clients":[{"id":"existing"}]},"streamSettings":{"network":"tcp","security":"reality"}}`)
	before := ProtectedHash([]json.RawMessage{reality})
	raws := []json.RawMessage{reality, b}
	if before != ProtectedHash(raws) {
		t.Fatal("Trojan contaminated Reality fingerprint")
	}
	id, e := Observe(raws, p, "192.0.2.1", c.Clients(p), sql.NullInt64{})
	if e != nil || id != 7 {
		t.Fatal(id, e)
	}
	for _, field := range []string{"flow", "realitySettings", "xhttpSettings"} {
		if strings.Contains(string(b), field) {
			t.Fatal("Reality/XHTTP field leaked into Trojan")
		}
	}
	if _, e = Observe(raws, p, "192.0.2.1", c.Clients(p), sql.NullInt64{Int64: 9, Valid: true}); e == nil {
		t.Fatal("replaced inbound adopted")
	}
	mutated := append([]Client{}, c.Clients(p)...)
	mutated[0].Password = strings.Repeat("z", 43)
	if _, e = Observe(raws, p, "192.0.2.1", mutated, sql.NullInt64{}); e == nil {
		t.Fatal("password drift accepted")
	}
	conflict := json.RawMessage(`{"id":12,"remark":"manual","protocol":"vless","port":8443,"enable":false}`)
	if _, e = Observe(append(raws, conflict), p, "192.0.2.1", c.Clients(p), sql.NullInt64{}); e == nil {
		t.Fatal("disabled foreign port was stolen")
	}
}
func testChain(t *testing.T, ip string, expiry time.Time) (string, *x509.CertPool) {
	t.Helper()
	now := time.Now()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Hour), NotAfter: now.Add(30 * 24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	root, e := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	rootCert, e := x509.ParseCertificate(root)
	if e != nil {
		t.Fatal(e)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: now.Add(-time.Hour), NotAfter: expiry, IPAddresses: []net.IP{net.ParseIP(ip)}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, leaf, rootCert, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	roots := x509.NewCertPool()
	roots.AddCert(rootCert)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})) + string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root})), roots
}
func TestTrojanCertificateRequiresTrustedMatchingIPAndLifetime(t *testing.T) {
	chain, roots := testChain(t, "192.0.2.1", time.Now().Add(6*24*time.Hour))
	if _, e := VerifyCertificate(chain, "192.0.2.1", time.Now(), roots); e != nil {
		t.Fatal(e)
	}
	if _, e := VerifyCertificate(chain, "192.0.2.2", time.Now(), roots); e == nil {
		t.Fatal("wrong IP accepted")
	}
	if _, e := VerifyCertificate(chain, "192.0.2.1", time.Now(), nil); e == nil {
		t.Fatal("untrusted certificate accepted")
	}
	if _, e := VerifyCertificate(chain, "192.0.2.1", time.Now().Add(6*24*time.Hour), roots); e == nil {
		t.Fatal("expired certificate accepted")
	}
	short, r := testChain(t, "192.0.2.1", time.Now().Add(23*time.Hour))
	if _, e := VerifyCertificate(short, "192.0.2.1", time.Now(), r); e == nil {
		t.Fatal("short-lived remainder accepted")
	}
}
