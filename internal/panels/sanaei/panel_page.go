package sanaei

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FetchPanelPage performs the same authenticated Sanaei
// session handshake as the API executor, but only GETs a
// panel UI path. It is intentionally read-only.
func (e SSHSessionExecutorV2) FetchPanelPage(
	ctx context.Context,
	pagePath string,
) ([]byte, int, error) {
	if e.Secrets == nil || e.AccountID == "" ||
		e.Username == "" || e.Port <= 0 ||
		e.PrivateKeySecretRef == "" ||
		e.PanelPasswordSecretRef == "" {
		return nil, 0, ErrSessionRequest
	}

	password, err := e.Secrets.Get(ctx, e.AccountID, e.PanelPasswordSecretRef)
	if err != nil {
		return nil, 0, err
	}
	defer wipeAuth(password)
	key, err := e.Secrets.Get(ctx, e.AccountID, e.PrivateKeySecretRef)
	if err != nil {
		return nil, 0, err
	}
	defer wipeAuth(key)

	login, err := json.Marshal(map[string]string{
		"username": e.Username,
		"password": string(password),
	})
	if err != nil {
		return nil, 0, ErrSessionRequest
	}

	base := e.BasePath
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	dial := strings.TrimSpace(e.DialHost)
	if dial == "" {
		dial = "127.0.0.1"
	}
	path := strings.TrimLeft(pagePath, "/")

	command := strings.Join([]string{
		"set -Eeuo pipefail",
		`COOKIE="$(mktemp)"; TOKEN="$(mktemp)"; OUT="$(mktemp)"`,
		`trap 'rm -f "$COOKIE" "$TOKEN" "$OUT"' EXIT`,
		"PORT=" + strconv.Itoa(e.Port),
		"BASE=" + shellQuoteSession(base),
		"DIAL=" + shellQuoteSession(dial),
		`curl -fsS -c "$COOKIE" -b "$COOKIE" "http://${DIAL}:${PORT}${BASE}csrf-token" -o "$TOKEN"`,
		`CSRF="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1])).get("obj",""))' "$TOKEN")"`,
		"LOGIN=" + shellQuoteSession(string(login)),
		`curl -fsS -c "$COOKIE" -b "$COOKIE" -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF" --data "$LOGIN" "http://${DIAL}:${PORT}${BASE}login" >/dev/null`,
		`CODE="$(curl -sS -o "$OUT" -w '%{http_code}' -c "$COOKIE" -b "$COOKIE" "http://${DIAL}:${PORT}${BASE}` + path + `")"`,
		`gzip -c "$OUT" | base64 -w0`,
		`printf '\n%s\n' "$CODE"`,
	}, "\n")

	result, err := e.SSH.RunDetailed(ctx, e.Target, key, command)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: page ssh: %v diagnostic=%q", ErrSessionRequest, err, result.Stderr)
	}
	trimmed := strings.TrimSpace(result.Stdout)
	cut := strings.LastIndex(trimmed, "\n")
	if cut < 0 {
		return nil, 0, ErrSessionRequest
	}
	payload := strings.TrimSpace(trimmed[:cut])
	code, err := strconv.Atoi(strings.TrimSpace(trimmed[cut+1:]))
	if err != nil {
		return nil, 0, ErrSessionRequest
	}
	body, err := decodeV2Body(payload)
	if err != nil {
		return nil, 0, err
	}
	return body, code, nil
}
