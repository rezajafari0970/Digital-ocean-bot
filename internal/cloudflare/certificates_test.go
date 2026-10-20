package cloudflare

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestOriginCertificateTrustNamesExpiryAndCSRBinding(t *testing.T) {
	now := time.Now()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	caKey, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Fixture CA"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(30 * 24 * time.Hour)}
	der, e := x509.CreateCertificate(rand.Reader, root, root, &caKey.PublicKey, caKey)
	if e != nil {
		t.Fatal(e)
	}
	rootPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	csrDER, e := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{DNSNames: []string{"*.example.com"}}, key)
	if e != nil {
		t.Fatal(e)
	}
	csr := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))
	makeCert := func(names []string, until time.Time) string {
		t.Helper()
		leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: names, NotBefore: now.Add(-time.Hour), NotAfter: until, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		b, e := x509.CreateCertificate(rand.Reader, leaf, root, &key.PublicKey, caKey)
		if e != nil {
			t.Fatal(e)
		}
		return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: b}))
	}
	valid := makeCert([]string{"*.example.com"}, now.Add(7*24*time.Hour))
	if _, e = VerifyCertificate(valid, csr, []string{"*.example.com"}, rootPEM, now); e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		cert  string
		names []string
		root  []byte
	}{
		{valid, []string{"*.other.com"}, rootPEM},
		{valid, []string{"*.deep.example.com"}, rootPEM},
		{valid, []string{"*.example.com"}, originRoot},
		{makeCert([]string{"*.example.com"}, now.Add(time.Hour)), []string{"*.example.com"}, rootPEM},
	} {
		if _, e = VerifyCertificate(c.cert, csr, c.names, c.root, now); e == nil {
			t.Fatal("invalid certificate accepted")
		}
	}
	wrong, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	b, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, wrong)
	if _, e = VerifyCertificate(valid, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: b})), []string{"*.example.com"}, rootPEM, now); e == nil {
		t.Fatal("wrong CSR key accepted")
	}
}
