package residentialsync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/protocolprobe"
)

// A real TLS/XUDP donor with its real category policy fronts a loopback-only
// SOCKS fixture. No public network or ad HTTP request participates in this test.
func TestNativeProtocolManagedRelayRespectsDestinationPolicy(t *testing.T) {
	binaryPath := os.Getenv("XRAY_TEST_BINARY")
	if binaryPath == "" {
		t.Skip("pinned native core required")
	}
	tcp, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	udp, e := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if e != nil {
		tcp.Close()
		t.Fatal(e)
	}
	t.Cleanup(func() { tcp.Close(); udp.Close() })
	go func() {
		for {
			conn, err := tcp.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(35 * time.Second))
				h := make([]byte, 2)
				if _, e := io.ReadFull(c, h); e != nil {
					return
				}
				methods := make([]byte, int(h[1]))
				if _, e := io.ReadFull(c, methods); e != nil {
					return
				}
				_, _ = c.Write([]byte{5, 0})
				request := make([]byte, 4)
				if _, e := io.ReadFull(c, request); e != nil {
					return
				}
				if request[1] != 3 {
					return
				}
				size := 6
				switch request[3] {
				case 1:
				case 4:
					size = 18
				case 3:
					b := make([]byte, 1)
					if _, e := io.ReadFull(c, b); e != nil {
						return
					}
					size = int(b[0]) + 2
				default:
					return
				}
				if _, e := io.ReadFull(c, make([]byte, size)); e != nil {
					return
				}
				reply := []byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}
				binary.BigEndian.PutUint16(reply[8:], uint16(udp.LocalAddr().(*net.UDPAddr).Port))
				_, _ = c.Write(reply)
				_, _ = io.Copy(io.Discard, c)
			}(conn)
		}
	}()
	go func() {
		data := make([]byte, 4096)
		for {
			n, origin, err := udp.ReadFromUDP(data)
			if err != nil {
				return
			}
			if n < 10 || !bytes.Equal(data[:3], []byte{0, 0, 0}) {
				continue
			}
			offset := 3 + socksAddressSize(data[3:n])
			if offset < 10 || n-offset < 23 {
				continue
			}
			request := data[offset:n]
			if !bytes.Equal(request[1:5], []byte{0xda, 0xda, 0xda, 0xda}) {
				continue
			}
			answer := []byte{0x80, 0, 0, 0, 0, 8}
			answer = append(answer, request[15:23]...)
			answer = append(answer, 8)
			answer = append(answer, request[6:14]...)
			answer = append(answer, 0, 0, 0, 1)
			packet := append(append([]byte{}, data[:offset]...), answer...)
			_, _ = udp.WriteToUDP(packet, origin)
		}
	}()
	credential, e := newRelayCredential()
	if e != nil {
		t.Fatal(e)
	}
	donorID := "00000000-0000-4000-8000-000000000070"
	donorHost := "127.0.0.2"
	donorPort := relayFreePort(t)
	p := routePolicy{DNSResilience: true, StrictAllowlist: true, Harden: true, AdsOnly: true, Residential: true, Configured: 1, StableFingerprint: true,
		Proxies:    []rp{{ID: "leaf", Tag: "residential-ads-leaf", Type: "socks5", Host: "127.0.0.1", Port: tcp.Addr().(*net.TCPAddr).Port, UDPRequired: true, QUICVerified: true}},
		RelayInlet: &relayInlet{Panel: donorID, Host: donorHost, Port: donorPort, Credential: credential, Sources: []string{"127.0.0.1/32"}}}
	donor, e := buildSettings(map[string]any{"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}}, "routing": map[string]any{"domainStrategy": "AsIs", "rules": []any{}}}, nil, nil, p)
	if e != nil {
		t.Fatal(e)
	}
	relayCore(t, donor, net.JoinHostPort(donorHost, fmtSprint(donorPort)))
	proxy, e := managedSOCKS(donorID, "203.0.113.9", "fixture", 8080, credential)
	if e != nil {
		t.Fatal(e)
	}
	proxy.Host = donorHost
	proxy.Port = donorPort
	candidates, e := protocolCandidates([]rp{proxy})
	if e != nil {
		t.Fatal(e)
	}
	// Identical native transport, but the old generic destination probe is denied.
	generic := candidates[0]
	generic.ID = "generic"
	generic.Driver = protocolprobe.DriverSOCKS5
	generic.Digest = protocolprobe.Fingerprint(generic.Driver, generic.Outbound)
	candidates = append(candidates, generic)
	raw, e := os.ReadFile(binaryPath)
	if e != nil {
		t.Fatal(e)
	}
	digest := sha256.Sum256(raw)
	input, _ := json.Marshal(protocolprobe.Request{Candidates: candidates, CoreHashes: []string{hex.EncodeToString(digest[:])}})
	script := `
import pathlib,os,json,types,sys
ns={'__name__':'probe_fixture'}
exec(pathlib.Path('../../protocolprobe/probe.py').read_text(),ns)
core=pathlib.Path(os.environ['XRAY_TEST_BINARY'])
ns['pathlib']=types.SimpleNamespace(Path=lambda _:core)
request=json.load(sys.stdin)
result=ns['probe'](request)
original=ns['exchange']
ns['exchange']=lambda local,host,port,payload,validator:original(local,host,8443 if port==443 else port,payload,validator)
unmeasured=ns['probe'](dict(request,candidates=request['candidates'][:1]))
assert not unmeasured['results'][0]['quic'], 'unmeasured UDP port escaped donor guard'
print(json.dumps(result))
`
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "timeout", "--kill-after=3s", "40s", "python3", "-c", script)
	cmd.Stdin = bytes.NewReader(input)
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("native probe: %v %s", e, out)
	}
	observations, e := protocolprobe.Decode(string(out), candidates)
	if e != nil {
		t.Fatal(e, string(out))
	}
	if observations[0].Code != "ok" || observations[0].DNS || !observations[0].QUIC {
		t.Fatal("valid managed QUIC was rejected or DNS falsely certified", observations)
	}
	if observations[1].Code != "datagram_unverified" || observations[1].DNS || observations[1].QUIC {
		t.Fatal("donor destination policy was bypassed", observations)
	}
}
