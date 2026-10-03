package healthverify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"net"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalid = errors.New("invalid xray health contract")
	ErrFailed  = errors.New("xray active health verification failed")
)

type RealityClient struct {
	UUID        string
	Host        string
	Port        int
	SNI         string
	PublicKey   string
	ShortID     string
	Flow        string
	Fingerprint string
}

type Result struct {
	Healthy bool
	ExitIP  string
	Latency time.Duration
}

type Runner interface {
	Run(context.Context, string) (string, error)
}

func Probe(ctx context.Context, r Runner, c RealityClient) (Result, error) {
	if r == nil || c.UUID == "" || c.Host == "" || c.Port < 1 || c.Port > 65535 || c.SNI == "" || c.PublicKey == "" || c.ShortID == "" {
		return Result{}, ErrInvalid
	}
	if net.ParseIP(c.Host) == nil || strings.ContainsAny(c.SNI, "/\\ \t\r\n") {
		return Result{}, ErrInvalid
	}
	if c.Flow == "" {
		c.Flow = "xtls-rprx-vision"
	}
	if c.Fingerprint == "" {
		c.Fingerprint = "chrome"
	}
	socksPort := 30000 + int(crc32.ChecksumIEEE([]byte(c.UUID))%20000)
	cfg := map[string]any{
		"log":       map[string]any{"loglevel": "warning"},
		"inbounds":  []any{map[string]any{"listen": "127.0.0.1", "port": socksPort, "protocol": "socks", "settings": map[string]any{"auth": "noauth", "udp": true}}},
		"outbounds": []any{map[string]any{"protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{"address": c.Host, "port": c.Port, "users": []any{map[string]any{"id": c.UUID, "encryption": "none", "flow": c.Flow}}}}}, "streamSettings": map[string]any{"network": "tcp", "security": "reality", "realitySettings": map[string]any{"serverName": c.SNI, "fingerprint": c.Fingerprint, "publicKey": c.PublicKey, "shortId": c.ShortID, "spiderX": "/"}}}},
	}
	raw, e := json.Marshal(cfg)
	if e != nil {
		return Result{}, ErrInvalid
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	started := time.Now()
	out, e := r.Run(ctx, command(encoded, c.Host, c.Port, socksPort))
	if e != nil {
		return Result{}, ErrFailed
	}
	exitIP := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "ip=") {
			exitIP = strings.TrimSpace(strings.TrimPrefix(line, "ip="))
		}
	}
	if net.ParseIP(exitIP) == nil || exitIP != c.Host || !strings.Contains(out, "TUNNEL_HTTPS=PASS") {
		return Result{}, ErrFailed
	}
	return Result{Healthy: true, ExitIP: exitIP, Latency: time.Since(started)}, nil
}

func command(cfg, host string, port, socksPort int) string {
	return fmt.Sprintf(`set -Eeuo pipefail
SOCKS_PORT=%s
XRAY="$(command -v xray || true)"
[ -n "$XRAY" ] || XRAY=/usr/local/x-ui/bin/xray-linux-amd64
[ -x "$XRAY" ]
TMP="$(mktemp -d)"
XPID=""
cleanup(){ [ -z "$XPID" ] || kill "$XPID" 2>/dev/null || true; rm -rf "$TMP"; }
trap cleanup EXIT
printf '%%s' '%s' | base64 -d >"$TMP/config.json"
"$XRAY" run -test -config "$TMP/config.json" >/dev/null
timeout 7 bash -c 'exec 3<>/dev/tcp/%s/%s'
"$XRAY" run -config "$TMP/config.json" >"$TMP/xray.log" 2>&1 & XPID=$!
sleep 2
kill -0 "$XPID"
curl -fsS --max-time 15 --socks5-hostname "127.0.0.1:$SOCKS_PORT" https://www.cloudflare.com/cdn-cgi/trace >"$TMP/trace"
echo TUNNEL_HTTPS=PASS
grep -E '^(ip|loc|warp)=' "$TMP/trace"
`, strconv.Itoa(socksPort), cfg, host, strconv.Itoa(port))
}
