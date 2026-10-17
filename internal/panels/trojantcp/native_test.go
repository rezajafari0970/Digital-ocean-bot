package trojantcp

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTrojanTCPNativeTLSAuthentication(t *testing.T) {
	core := os.Getenv("XRAY_TEST_BINARY")
	if core == "" {
		t.Skip("XRAY_TEST_BINARY required")
	}
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(6 * 24 * time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	private, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if e = os.WriteFile(certPath, certificate, 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), 0600); e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	p := Policy{Port: port, Direct: true}
	c, e := NewCredentials("native")
	if e != nil {
		t.Fatal(e)
	}
	in, e := Build(p, "127.0.0.1", c.Clients(p))
	if e != nil {
		t.Fatal(e)
	}
	in["listen"] = "127.0.0.1"
	in["tag"] = "trojan-test"
	certs := in["streamSettings"].(map[string]any)["tlsSettings"].(map[string]any)["certificates"].([]any)
	certs[0].(map[string]any)["certificateFile"] = certPath
	certs[0].(map[string]any)["keyFile"] = keyPath
	// This isolated fixture explicitly permits its loopback HTTP origin. Production routing is untouched.
	cfg := map[string]any{"log": map[string]any{"loglevel": "debug"}, "inbounds": []any{in}, "outbounds": []any{map[string]any{"protocol": "freedom", "settings": map[string]any{"finalRules": []any{map[string]any{"action": "allow", "network": "tcp", "ip": []string{"127.0.0.1/32"}}}}}}}
	raw, _ := json.Marshal(cfg)
	file := filepath.Join(dir, "config.json")
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	if out, e := exec.Command(core, "run", "-test", "-config", file).CombinedOutput(); e != nil {
		t.Fatalf("native config rejected: %v %s", e, out)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, core, "run", "-config", file)
	var nativeLog strings.Builder
	cmd.Stdout = &nativeLog
	cmd.Stderr = &nativeLog
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
		if t.Failed() {
			t.Log(nativeLog.String())
		}
	}()
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	deadline := time.Now().Add(5 * time.Second)
	for {
		cn, e := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if e == nil {
			cn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("core did not bind")
		}
		time.Sleep(20 * time.Millisecond)
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "trojan-native-ok") }))
	defer origin.Close()
	_, ps, e := net.SplitHostPort(strings.TrimPrefix(origin.URL, "http://"))
	if e != nil {
		t.Fatal(e)
	}
	targetPort, _ := strconv.Atoi(ps)
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(certificate)
	fetch := func(password string, root *x509.CertPool) (string, error) {
		cn, e := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", address, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: "127.0.0.1", RootCAs: root})
		if e != nil {
			return "", e
		}
		defer cn.Close()
		cn.SetDeadline(time.Now().Add(time.Second))
		h := sha256.Sum224([]byte(password))
		frame := []byte(hex.EncodeToString(h[:]) + "\r\n")
		frame = append(frame, 1, 1, 127, 0, 0, 1)
		frame = binary.BigEndian.AppendUint16(frame, uint16(targetPort))
		frame = append(frame, '\r', '\n')
		frame = append(frame, []byte("GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n")...)
		if _, e = cn.Write(frame); e != nil {
			return "", e
		}
		res, e := http.ReadResponse(bufio.NewReader(cn), nil)
		if e != nil {
			return "", e
		}
		defer res.Body.Close()
		b, e := io.ReadAll(res.Body)
		return string(b), e
	}
	if got, e := fetch(c.Direct.Password, roots); e != nil || got != "trojan-native-ok" {
		t.Fatal("native Trojan connection failed", e, got)
	}
	if got, e := fetch(strings.Repeat("x", 43), roots); e == nil && got == "trojan-native-ok" {
		t.Fatal("wrong password accepted")
	}
	if _, e := fetch(c.Direct.Password, nil); e == nil {
		t.Fatal("untrusted TLS accepted")
	}
	if got, e := fetch(c.Direct.Password, roots); e != nil || got != "trojan-native-ok" {
		t.Fatal("invalid login damaged valid client", fmt.Sprint(e))
	}
}
