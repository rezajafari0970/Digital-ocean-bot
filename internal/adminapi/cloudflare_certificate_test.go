package adminapi

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/cloudflare"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"math/big"
	"testing"
	"time"
)

type cfCertDriver struct {
	cfDriverFixture
	ca           *x509.Certificate
	key          *ecdsa.PrivateKey
	certificates []cloudflare.Certificate
	issues       int
	lost         bool
	commit       bool
}

func (d *cfCertDriver) Certificates(context.Context, string) ([]cloudflare.Certificate, error) {
	return d.certificates, nil
}
func (d *cfCertDriver) Issue(_ context.Context, csr string, names []string) (cloudflare.Certificate, error) {
	d.issues++
	b, _ := pem.Decode([]byte(csr))
	request, e := x509.ParseCertificateRequest(b.Bytes)
	if e != nil {
		return cloudflare.Certificate{}, e
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(int64(d.issues + 1)), DNSNames: names, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(7 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, leaf, d.ca, request.PublicKey, d.key)
	if e != nil {
		return cloudflare.Certificate{}, e
	}
	c := cloudflare.Certificate{ID: "fixture-issued", CSR: csr, Certificate: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), Hostnames: names, ExpiresOn: leaf.NotAfter}
	if d.commit {
		d.certificates = append(d.certificates, c)
	}
	if d.lost {
		return cloudflare.Certificate{}, errors.New("lost response")
	}
	return c, nil
}

type cfNodeFixture struct {
	csrs      map[string]string
	installed string
	installs  int
	broken    bool
}

func (n *cfNodeFixture) RunInput(_ context.Context, _ provisioning.Target, _ []byte, command string, input []byte) (provisioning.CommandResult, error) {
	var p struct {
		Action      string   `json:"action"`
		Generation  string   `json:"generation"`
		Names       []string `json:"names"`
		Certificate string   `json:"certificate"`
	}
	if json.Unmarshal(input, &p) != nil {
		return provisioning.CommandResult{}, errors.New("input")
	}
	var out any
	switch p.Action {
	case "prepare":
		if n.csrs == nil {
			n.csrs = map[string]string{}
		}
		if n.csrs[p.Generation] == "" {
			key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			b, e := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{DNSNames: p.Names}, key)
			if e != nil {
				return provisioning.CommandResult{}, e
			}
			n.csrs[p.Generation] = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: b}))
		}
		out = map[string]string{"csr": n.csrs[p.Generation]}
	case "install":
		b, e := base64.StdEncoding.DecodeString(p.Certificate)
		if e != nil {
			return provisioning.CommandResult{}, e
		}
		sum := sha256.Sum256(b)
		n.installed = hex.EncodeToString(sum[:])
		n.installs++
		n.broken = false
		out = map[string]any{"installed": true, "sha256": n.installed}
	case "inspect":
		if n.broken {
			return provisioning.CommandResult{}, errors.New("missing local key")
		}
		out = map[string]string{"sha256": n.installed}
	default:
		return provisioning.CommandResult{}, errors.New("unexpected node action")
	}
	b, _ := json.Marshal(out)
	return provisioning.CommandResult{Stdout: string(b)}, nil
}
func TestCloudflareCertificateLostResponseRenewalAndNodeRepair(t *testing.T) {
	db := adminTestDB(t)
	account, _, panel := cfSeed(t, db)
	ctx := context.Background()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Fixture CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(30 * 24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, e := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if e != nil {
		t.Fatal(e)
	}
	root := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	d := &cfCertDriver{ca: ca, key: caKey, lost: true, commit: true}
	node := &cfNodeFixture{}
	svc := cloudflare.Service{DB: db, CertificateRoots: root, Factory: func(context.Context, string) (cloudflare.Driver, error) { return d, nil }}
	ensure := func() error {
		_, e := svc.EnsureCertificate(ctx, account, panel, node, provisioning.Target{}, nil)
		return e
	}
	if ensure() == nil || d.issues != 1 {
		t.Fatal("lost issuance accepted")
	}
	sqlMust(t, db, "UPDATE cloudflare_zones SET dns_error_code='cloudflare_dns_read_permission_denied'")
	if e = ensure(); e != nil {
		t.Fatal(e)
	}
	if d.issues != 1 || node.installs != 1 {
		t.Fatal("ambiguous issue replayed", d.issues, node.installs)
	}
	if e = ensure(); e != nil || node.installs != 1 {
		t.Fatal("valid installed certificate replaced", e)
	}
	sqlMust(t, db, "UPDATE cloudflare_zones SET dns_error_code=''")
	d.lost = false
	node.broken = true
	if e = ensure(); e != nil {
		t.Fatal(e)
	}
	if d.issues != 2 || node.installs != 2 || len(node.csrs) != 2 {
		t.Fatal("node key repair did not create independent generation")
	}
	sqlMust(t, db, "UPDATE cloudflare_certificates SET not_after=now()+interval '24 hours'")
	if e = ensure(); e != nil {
		t.Fatal(e)
	}
	if d.issues != 3 || node.installs != 3 {
		t.Fatal("renewal not independent of DNS rotation")
	}
	// A definitely unobserved old CSR may be replaced after two spaced reads,
	// with its intent retained. It is never blindly submitted again.
	sqlMust(t, db, "UPDATE cloudflare_certificates SET state='SENT',csr='unseen-csr',updated_at=now()-interval '6 minutes',negative_observations=1,last_observed_at=now()-interval '2 minutes'")
	before := d.issues
	if ensure() == nil {
		t.Fatal("unconfirmed issue accepted")
	}
	if d.issues != before {
		t.Fatal("blind CSR replay")
	}
	var state string
	var history int
	db.QueryRow("SELECT state FROM cloudflare_certificates").Scan(&state)
	db.QueryRow("SELECT count(*) FROM cloudflare_certificate_history").Scan(&history)
	if state != "PREPARED" || history != 1 {
		t.Fatal("new intent not journaled", state, history)
	}
	if e = ensure(); e != nil {
		t.Fatal(e)
	}
	if d.issues != before+1 {
		t.Fatal("fresh CSR recovery missing")
	}
}
func TestCloudflareDNSCleanupSurvivesPanelPurge(t *testing.T) {
	db := adminTestDB(t)
	_, _, panel := cfSeed(t, db)
	ctx := context.Background()
	d := &cfDriverFixture{records: map[string]cloudflare.Record{}}
	svc := cloudflare.Service{DB: db, Factory: func(context.Context, string) (cloudflare.Driver, error) { return d, nil }}
	if e := svc.RunOne(ctx); e != nil {
		t.Fatal(e)
	}
	sqlMust(t, db, "DELETE FROM panel_instances WHERE id=$1", panel)
	for i := 0; i < 2; i++ {
		sqlMust(t, db, "UPDATE cloudflare_accounts SET next_attempt_at=now()")
		sqlMust(t, db, "UPDATE cloudflare_records SET next_attempt_at=now()")
		if e := svc.RunOne(ctx); e != nil {
			t.Fatal(e)
		}
	}
	var n int
	db.QueryRow("SELECT count(*) FROM cloudflare_records WHERE state='DELETED'").Scan(&n)
	if n != 1 || d.deletes != 1 {
		t.Fatal("orphaned DNS", n, d.deletes)
	}
}
