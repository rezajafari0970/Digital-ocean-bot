package sanaei

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

// SSHSessionExecutorV2 uses explicit wire markers.
//
// It is intentionally separate from SSHSessionExecutor
// until integration tests prove the new transport.
type SSHSessionExecutorV2 struct {
	SSH provisioning.SSHClient

	Target provisioning.Target

	PrivateKeySecretRef string

	PanelPasswordSecretRef string

	AccountID string

	Username string

	Port int

	BasePath string

	DialHost string

	Secrets SSHSecretReader
}

const (
	v2Begin = "DOB_RESPONSE_BEGIN"

	v2End = "DOB_RESPONSE_END"

	v2Status = "DOB_STATUS="
)

func extractV2Wire(
	stdout string,
) (
	string,
	int,
	error,
) {

	beginToken :=
		v2Begin + "\n"

	endToken :=
		"\n" + v2End + "\n"

	begin :=
		strings.Index(
			stdout,
			beginToken,
		)

	if begin < 0 {
		return "", 0,
			fmt.Errorf(
				"%w: v2 begin marker missing",
				ErrSessionRequest,
			)
	}

	begin += len(
		beginToken,
	)

	rest :=
		stdout[begin:]

	end :=
		strings.Index(
			rest,
			endToken,
		)

	if end < 0 {
		return "", 0,
			fmt.Errorf(
				"%w: v2 end marker missing",
				ErrSessionRequest,
			)
	}

	payload :=
		strings.TrimSpace(
			rest[:end],
		)

	trailer :=
		rest[end+len(endToken):]

	statusPos :=
		strings.Index(
			trailer,
			v2Status,
		)

	if statusPos < 0 {
		return "", 0,
			fmt.Errorf(
				"%w: v2 status missing",
				ErrSessionRequest,
			)
	}

	statusText :=
		strings.TrimSpace(
			trailer[statusPos+
				len(v2Status):],
		)

	// Only first line belongs to status.
	if i :=
		strings.IndexByte(
			statusText,
			'\n',
		); i >= 0 {

		statusText =
			strings.TrimSpace(
				statusText[:i],
			)
	}

	statusCode, err :=
		strconv.Atoi(
			statusText,
		)

	if err != nil {
		return "", 0,
			fmt.Errorf(
				"%w: v2 invalid status",
				ErrSessionRequest,
			)
	}

	return payload,
		statusCode,
		nil
}

func decodeV2Body(
	payload string,
) ([]byte, error) {

	compressed, err :=
		base64.StdEncoding.DecodeString(
			payload,
		)

	if err != nil {
		return nil,
			fmt.Errorf(
				"%w: v2 base64",
				ErrSessionRequest,
			)
	}

	zr, err :=
		gzip.NewReader(
			bytes.NewReader(
				compressed,
			),
		)

	if err != nil {
		return nil,
			fmt.Errorf(
				"%w: v2 gzip",
				ErrSessionRequest,
			)
	}

	defer zr.Close()

	body, err :=
		io.ReadAll(zr)

	if err != nil {
		return nil,
			fmt.Errorf(
				"%w: v2 read",
				ErrSessionRequest,
			)
	}

	return body, nil
}

func (e SSHSessionExecutorV2) Do(
	ctx context.Context,
	req SessionRequest,
) (
	SessionResponse,
	error,
) {

	if e.Secrets == nil ||
		e.AccountID == "" ||
		e.Username == "" ||
		e.Port <= 0 ||
		e.PrivateKeySecretRef == "" ||
		e.PanelPasswordSecretRef == "" {

		return SessionResponse{},
			ErrSessionRequest
	}

	method :=
		strings.ToUpper(
			strings.TrimSpace(
				req.Method,
			),
		)

	if method == "POST" {
		return e.doPOST(ctx, req)
	}

	if method != "GET" {
		return SessionResponse{},
			fmt.Errorf(
				"%w: v2 unsupported method",
				ErrSessionRequest,
			)
	}

	password, err :=
		e.Secrets.Get(
			ctx,
			e.AccountID,
			e.PanelPasswordSecretRef,
		)

	if err != nil {
		return SessionResponse{}, err
	}

	defer wipeAuth(password)

	privateKey, err :=
		e.Secrets.Get(
			ctx,
			e.AccountID,
			e.PrivateKeySecretRef,
		)

	if err != nil {
		return SessionResponse{}, err
	}

	defer wipeAuth(privateKey)

	loginPayload, err :=
		json.Marshal(
			map[string]string{
				"username": e.Username,

				"password": string(password),
			},
		)

	if err != nil {
		return SessionResponse{},
			ErrSessionRequest
	}

	basePath :=
		e.BasePath

	if !strings.HasPrefix(
		basePath,
		"/",
	) {
		basePath =
			"/" + basePath
	}

	if !strings.HasSuffix(
		basePath,
		"/",
	) {
		basePath += "/"
	}

	dial :=
		strings.TrimSpace(
			e.DialHost,
		)

	if dial == "" {
		dial =
			"127.0.0.1"
	}

	path :=
		strings.TrimLeft(
			req.Path,
			"/",
		)

	timeoutSeconds :=
		req.TimeoutSeconds

	if timeoutSeconds == 0 {
		timeoutSeconds = 8
	}

	if timeoutSeconds < 1 ||
		timeoutSeconds > 60 {

		return SessionResponse{},
			ErrSessionRequest
	}

	command :=
		strings.Join(
			[]string{
				`set -Eeuo pipefail`,

				`COOKIE="$(mktemp)"`,

				`TOKEN_JSON="$(mktemp)"`,

				`OUTPUT="$(mktemp)"`,

				`cleanup(){ rm -f "$COOKIE" "$TOKEN_JSON" "$OUTPUT"; }`,

				`trap cleanup EXIT`,

				`PORT=` +
					strconv.Itoa(
						e.Port,
					),

				`BASE=` +
					shellQuoteSession(
						basePath,
					),

				`DIAL=` +
					shellQuoteSession(
						dial,
					),

				`curl -fsS --max-time 8 ` +
					`-c "$COOKIE" -b "$COOKIE" ` +
					`"http://${DIAL}:${PORT}${BASE}csrf-token" ` +
					`-o "$TOKEN_JSON"`,

				`CSRF="$(python3 -c '` +
					`import json,sys;` +
					`print(json.load(open(sys.argv[1])).get("obj",""))` +
					`' "$TOKEN_JSON")"`,

				`test -n "$CSRF"`,

				`LOGIN=` +
					shellQuoteSession(
						string(
							loginPayload,
						),
					),

				`curl -fsS --max-time 8 ` +
					`-c "$COOKIE" -b "$COOKIE" ` +
					`-H 'Content-Type: application/json' ` +
					`-H "X-CSRF-Token: $CSRF" ` +
					`--data "$LOGIN" ` +
					`"http://${DIAL}:${PORT}${BASE}login" ` +
					`>/dev/null`,

				`CODE="$(curl -sS --max-time ` +
					strconv.Itoa(
						timeoutSeconds,
					) +
					` -o "$OUTPUT" ` +
					`-w '%{http_code}' ` +
					`-c "$COOKIE" -b "$COOKIE" ` +
					`-H "X-CSRF-Token: $CSRF" ` +
					`-X GET ` +
					`"http://${DIAL}:${PORT}${BASE}` +
					path +
					`")"`,

				`printf '%s\n' ` +
					shellQuoteSession(
						v2Begin,
					),

				`gzip -c "$OUTPUT" | base64 -w0`,

				`printf '\n%s\n' ` +
					shellQuoteSession(
						v2End,
					),

				`printf '%s%s\n' ` +
					shellQuoteSession(
						v2Status,
					) +
					` "$CODE"`,
			},
			"\n",
		)

	result, err :=
		e.SSH.RunDetailed(
			ctx,
			e.Target,
			privateKey,
			command,
		)

	if err != nil {
		return SessionResponse{},
			fmt.Errorf(
				"%w: v2 ssh: %v diagnostic=%q",
				ErrSessionRequest,
				err,
				result.Stderr,
			)
	}

	payload,
		statusCode,
		err :=
		extractV2Wire(
			result.Stdout,
		)

	if err != nil {
		return SessionResponse{}, err
	}

	body, err :=
		decodeV2Body(
			payload,
		)

	if err != nil {
		return SessionResponse{}, err
	}

	return SessionResponse{
		StatusCode: statusCode,

		Body: body,
	}, nil
}
