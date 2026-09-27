package sanaei

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

type SSHSecretReader interface {
	Get(
		context.Context,
		string,
		string,
	) ([]byte, error)
}

type SSHSessionExecutor struct {
	SSH provisioning.SSHClient

	Target provisioning.Target

	PrivateKeySecretRef    string
	PanelPasswordSecretRef string

	AccountID string
	Username  string

	Port     int
	BasePath string
	DialHost string

	Secrets SSHSecretReader
}

func shellQuoteSession(s string) string {
	return "'" +
		strings.ReplaceAll(
			s,
			"'",
			"'\"'\"'",
		) +
		"'"
}

func (e SSHSessionExecutor) Do(
	ctx context.Context,
	req SessionRequest,
) (SessionResponse, error) {

	if e.Secrets == nil ||
		e.AccountID == "" ||
		e.Port <= 0 ||
		e.Username == "" ||
		e.PanelPasswordSecretRef == "" ||
		e.PrivateKeySecretRef == "" {

		return SessionResponse{},
			ErrSessionRequest
	}

	password, err := e.Secrets.Get(
		ctx,
		e.AccountID,
		e.PanelPasswordSecretRef,
	)

	if err != nil {
		return SessionResponse{}, err
	}

	defer wipeAuth(password)

	privateKey, err := e.Secrets.Get(
		ctx,
		e.AccountID,
		e.PrivateKeySecretRef,
	)

	if err != nil {
		return SessionResponse{}, err
	}

	defer wipeAuth(privateKey)

	loginPayload, err := json.Marshal(
		map[string]string{
			"username": e.Username,
			"password": string(password),
		},
	)

	if err != nil {
		return SessionResponse{},
			ErrSessionRequest
	}

	body64 := base64.StdEncoding.EncodeToString(
		req.Body,
	)

	method := strings.ToUpper(
		strings.TrimSpace(req.Method),
	)

	timeoutSeconds := req.TimeoutSeconds
	if timeoutSeconds == 0 {
		timeoutSeconds = 8
	}
	if timeoutSeconds < 1 || timeoutSeconds > 60 {
		return SessionResponse{}, ErrSessionRequest
	}

	contentType := strings.TrimSpace(req.ContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	switch contentType {
	case "application/json", "application/x-www-form-urlencoded":
	default:
		return SessionResponse{}, ErrSessionRequest
	}

	switch method {
	case "GET", "POST":
	default:
		return SessionResponse{},
			ErrSessionRequest
	}

	path := strings.TrimLeft(
		req.Path,
		"/",
	)

	basePath := e.BasePath

	if !strings.HasPrefix(
		basePath,
		"/",
	) {
		basePath = "/" + basePath
	}

	if !strings.HasSuffix(
		basePath,
		"/",
	) {
		basePath += "/"
	}

	var bodyArgument string

	if len(req.Body) > 0 {
		bodyArgument =
			`--data-binary "$BODY"`
	}

	var responseFilter string
	if path == "panel/api/server/scanRealityTargets" {
		compact := `import json,sys;p=sys.argv[1];d=json.load(open(p));xs=d.get("obj") or [];keep=("target","host","port","feasible","tls13","tlsVersion","h2","alpn","x25519","curveID","certValid","certChainValid","certSubject","serverNames","latencyMs","reason");d["obj"]=[{k:(v[:2] if k=="serverNames" and isinstance(v,list) else v) for k,v in x.items() if k in keep} for x in xs[:6]];open(p,"w").write(json.dumps(d,separators=(",",":")))`
		responseFilter =
			"python3 -c " +
				shellQuoteSession(compact) +
				` "$OUTPUT"`
	}

	command := strings.Join(
		[]string{
			`set -Eeuo pipefail`,
			`COOKIE="$(mktemp)"`,
			`TOKEN_JSON="$(mktemp)"`,
			`OUTPUT="$(mktemp)"`,
			`cleanup(){ rm -f "$COOKIE" "$TOKEN_JSON" "$OUTPUT"; }`,
			`trap cleanup EXIT`,

			`PORT=` + strconv.Itoa(e.Port),
			`BASE=` + shellQuoteSession(basePath),
			`DIAL=` + shellQuoteSession(
				func() string {
					if e.DialHost != "" {
						return e.DialHost
					}
					return "127.0.0.1"
				}(),
			),

			`curl -fsS --max-time 8 ` +
				`-c "$COOKIE" -b "$COOKIE" ` +
				`"http://${DIAL}:${PORT}${BASE}csrf-token" ` +
				`-o "$TOKEN_JSON"`,

			`CSRF="$(python3 -c '` +
				`import json,sys; ` +
				`print(json.load(open(sys.argv[1])).get("obj",""))` +
				`' "$TOKEN_JSON")"`,

			`test -n "$CSRF"`,

			`LOGIN=` +
				shellQuoteSession(
					string(loginPayload),
				),

			`curl -fsS --max-time 8 ` +
				`-c "$COOKIE" -b "$COOKIE" ` +
				`-H 'Content-Type: application/json' ` +
				`-H "X-CSRF-Token: $CSRF" ` +
				`--data "$LOGIN" ` +
				`"http://${DIAL}:${PORT}${BASE}login" ` +
				`>/dev/null`,

			`BODY_B64=` +
				shellQuoteSession(body64),

			`BODY="$(printf '%s' "$BODY_B64" | base64 -d)"`,

			`CODE="$(curl -sS --max-time ` + strconv.Itoa(timeoutSeconds) + ` ` +
				`-o "$OUTPUT" ` +
				`-w '%{http_code}' ` +
				`-c "$COOKIE" -b "$COOKIE" ` +
				`-H ` + shellQuoteSession("Content-Type: "+contentType) + ` ` +
				`-H "X-CSRF-Token: $CSRF" ` +
				`-X ` + shellQuoteSession(method) + ` ` +
				bodyArgument + ` ` +
				`"http://${DIAL}:${PORT}${BASE}` +
				path +
				`")"`,

			responseFilter,

			// Put the payload first and status last. SSH diagnostics retain the
			// tail, so the framing remains parseable even for large responses.
			`head -c 1048576 "$OUTPUT" | base64 -w0`,
			`printf '\n%s\n' "$CODE"`,
		},
		"\n",
	)

	result, err := e.SSH.RunDetailed(
		ctx,
		e.Target,
		privateKey,
		command,
	)

	output := result.Stdout + result.Stderr

	if err != nil {
		return SessionResponse{},
			fmt.Errorf(
				"%w: ssh command: %v; diagnostic=%q",
				ErrSessionRequest,
				err,
				result.Stderr,
			)
	}

	parts := strings.SplitN(output, "\n", 2)
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return SessionResponse{}, fmt.Errorf("%w: missing HTTP status", ErrSessionRequest)
	}
	if len(parts) == 1 {
		parts = append(parts, "")
	}

	statusCode, err := strconv.Atoi(
		strings.TrimSpace(parts[0]),
	)

	if err != nil {
		return SessionResponse{},
			fmt.Errorf(
				"%w: invalid HTTP status %q",
				ErrSessionRequest,
				parts[0],
			)
	}

	body, err := base64.StdEncoding.DecodeString(
		strings.TrimSpace(parts[1]),
	)

	if err != nil {
		return SessionResponse{},
			fmt.Errorf(
				"%w: invalid response encoding",
				ErrSessionRequest,
			)
	}

	return SessionResponse{
		StatusCode: statusCode,
		Body:       body,
	}, nil
}
