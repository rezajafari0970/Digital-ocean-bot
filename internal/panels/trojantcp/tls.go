package trojantcp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

//go:embed certificate.py
var certificateScript string

func CertificateCommand(panel, host string, port, panelPort int) (string, error) {
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil || panel == "" || ValidatePort(port, panelPort, nil, nil) != nil {
		return "", errors.New("trojan_invalid_certificate_target")
	}
	return certificateCommand(panel, host, port, panelPort)
}

// ProfileCertificateCommand accepts independently reserved listener ports.
func ProfileCertificateCommand(panel, host string, port, panelPort int) (string, error) {
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil || panel == "" || port < 1 || port > 65535 || port == 22 || port == 80 || port == 18080 || port == panelPort {
		return "", errors.New("profile_invalid_certificate_target")
	}
	return certificateCommand(panel, host, port, panelPort)
}
func certificateCommand(panel, host string, port, panelPort int) (string, error) {
	ip := net.ParseIP(host)
	plan, _ := json.Marshal(map[string]any{"panel": panel, "host": ip.String(), "port": port, "panel_port": panelPort})
	script := base64.StdEncoding.EncodeToString([]byte(certificateScript))
	encoded := base64.StdEncoding.EncodeToString(plan)
	// All dynamic content is base64, passed as data; no secret material is embedded.
	return fmt.Sprintf(`umask 077
python3 - <<'PYDOBTLS'
import base64,json,os,pathlib,subprocess,sys,tempfile
plan=json.loads(base64.b64decode('%s'))
root=pathlib.Path('/opt/dob-trojan')
owner={'panel':plan['panel'],'host':plan['host']}
if root.exists():
 if root.is_symlink() or not root.is_dir() or not (root/'owner.json').is_file() or json.loads((root/'owner.json').read_text())!=owner:
  print('{"error":"owner_mismatch"}');sys.exit(1)
else:
 root.mkdir(mode=0o700)
 (root/'owner.json').write_text(json.dumps(owner))
 os.chmod(root/'owner.json',0o600)
target=root/'renew.py'
if target.is_symlink():sys.exit(1)
fd,name=tempfile.mkstemp(dir=root,prefix='.script-')
with os.fdopen(fd,'wb') as f:
 f.write(base64.b64decode('%s'));f.flush();os.fsync(f.fileno())
os.chmod(name,0o700);os.replace(name,target)
raise SystemExit(subprocess.run(['/usr/bin/python3',str(target),'%s']).returncode)
PYDOBTLS`, encoded, script, encoded), nil
}

func VerifyCertificate(chain, host string, now time.Time, roots *x509.CertPool) (TLSProof, error) {
	var certs []*x509.Certificate
	raw := []byte(chain)
	for len(raw) > 0 {
		block, rest := pem.Decode(raw)
		if block == nil {
			break
		}
		raw = rest
		if block.Type != "CERTIFICATE" {
			return TLSProof{}, errors.New("trojan_invalid_certificate_chain")
		}
		c, e := x509.ParseCertificate(block.Bytes)
		if e != nil {
			return TLSProof{}, errors.New("trojan_invalid_certificate")
		}
		certs = append(certs, c)
	}
	if len(certs) == 0 || net.ParseIP(host) == nil || !certs[0].NotAfter.After(now.Add(24*time.Hour)) {
		return TLSProof{}, errors.New("trojan_certificate_lifetime")
	}
	intermediates := x509.NewCertPool()
	for _, c := range certs[1:] {
		intermediates.AddCert(c)
	}
	if _, e := certs[0].Verify(x509.VerifyOptions{DNSName: host, Roots: roots, Intermediates: intermediates, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); e != nil {
		return TLSProof{}, errors.New("trojan_certificate_untrusted")
	}
	return TLSProof{certs[0].NotAfter, Digest(certs[0].Raw)}, nil
}
func ProbeTLS(ctx context.Context, host string, port int) (TLSProof, error) {
	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}}
	c, e := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if e != nil {
		return TLSProof{}, errors.New("trojan_tls_handshake_failed")
	}
	defer c.Close()
	tc, ok := c.(*tls.Conn)
	if !ok {
		return TLSProof{}, errors.New("trojan_tls_connection")
	}
	state := tc.ConnectionState()
	if len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 {
		return TLSProof{}, errors.New("trojan_tls_unverified")
	}
	leaf := state.PeerCertificates[0]
	if !leaf.NotAfter.After(time.Now().Add(5 * time.Minute)) {
		return TLSProof{}, errors.New("trojan_tls_expiring")
	}
	return TLSProof{leaf.NotAfter, Digest(leaf.Raw)}, nil
}
func CertificateResult(raw, host string) (TLSProof, error) {
	var v struct {
		Certificate string
		Error       string
	}
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &v) != nil || v.Error != "" || v.Certificate == "" {
		return TLSProof{}, errors.New("trojan_certificate_provision_failed")
	}
	return VerifyCertificate(v.Certificate, host, time.Now(), nil)
}
