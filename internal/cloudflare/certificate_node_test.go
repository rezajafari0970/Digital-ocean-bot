package cloudflare

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCertificateNodeOwnershipAtomicInstallAndPrivateKeyRetention(t *testing.T) {
	if _, e := exec.LookPath("openssl"); e != nil {
		t.Skip("openssl required")
	}
	if _, e := exec.LookPath("python3"); e != nil {
		t.Skip("python3 required")
	}
	root := t.TempDir()
	script := strings.ReplaceAll(certificateNode, "/opt/dob-cloudflare", filepath.ToSlash(root))
	input := map[string]any{"account": "11111111-1111-4111-8111-111111111111", "panel": "22222222-2222-4222-8222-222222222222", "generation": "33333333-3333-4333-8333-333333333333", "names": []string{"*.example.com"}, "action": "prepare"}
	run := func() (map[string]any, error) {
		b, _ := json.Marshal(input)
		cmd := exec.Command("python3", "-c", script)
		cmd.Stdin = bytes.NewReader(b)
		out, e := cmd.Output()
		var v map[string]any
		if e == nil {
			e = json.Unmarshal(out, &v)
		}
		return v, e
	}
	result, e := run()
	if e != nil {
		t.Fatal("node prepare", e)
	}
	csrPEM := result["csr"].(string)
	if strings.Contains(csrPEM, "PRIVATE KEY") {
		t.Fatal("private key exported")
	}
	again, e := run()
	if e != nil || again["csr"] != csrPEM {
		t.Fatal("CSR changed after retry", e)
	}
	block, _ := pem.Decode([]byte(csrPEM))
	csr, e := x509.ParseCertificateRequest(block.Bytes)
	if e != nil || csr.CheckSignature() != nil {
		t.Fatal("CSR invalid", e)
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Fixture CA"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(30 * 24 * time.Hour)}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"*.example.com"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(7 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, leaf, ca, csr.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	input["action"] = "install"
	input["certificate"] = base64.StdEncoding.EncodeToString(cert)
	result, e = run()
	if e != nil || result["installed"] != true {
		t.Fatal("install", e)
	}
	localKey := filepath.Join(root, input["account"].(string), "current", "privkey.pem")
	info, e := os.Stat(localKey)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("key permissions", e)
	}
	input["action"] = "inspect"
	if _, e = run(); e != nil {
		t.Fatal("installed key inspection", e)
	}
	input["panel"] = "44444444-4444-4444-8444-444444444444"
	if _, e = run(); e == nil {
		t.Fatal("different panel adopted key")
	}
	input["panel"] = "22222222-2222-4222-8222-222222222222"
	if e = os.Remove(localKey); e != nil {
		t.Fatal(e)
	}
	if _, e = run(); e == nil {
		t.Fatal("lost key was not detected")
	}
}
