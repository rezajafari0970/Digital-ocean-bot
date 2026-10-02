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

func (e SSHSessionExecutorV2) doPOST(
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

	contentType :=
		strings.TrimSpace(
			req.ContentType,
		)

	if contentType == "" {
		contentType =
			"application/json"
	}

	switch contentType {

	case "application/json",
		"application/x-www-form-urlencoded":

	default:

		return SessionResponse{},
			fmt.Errorf(
				"%w: v2 unsupported content type",
				ErrSessionRequest,
			)
	}

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

	body64 :=
		base64.StdEncoding.EncodeToString(
			req.Body,
		)

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

				`BODY_B64=` +
					shellQuoteSession(
						body64,
					),

				`BODY="$(printf '%s' "$BODY_B64" | base64 -d)"`,

				`CODE="$(curl -sS --max-time ` +
					strconv.Itoa(
						timeoutSeconds,
					) +
					` -o "$OUTPUT" ` +
					`-w '%{http_code}' ` +
					`-c "$COOKIE" -b "$COOKIE" ` +
					`-H ` +
					shellQuoteSession(
						"Content-Type: "+
							contentType,
					) +
					` -H "X-CSRF-Token: $CSRF" ` +
					`-X POST ` +
					`--data-binary "$BODY" ` +
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
				"%w: v2 post ssh: %w diagnostic=%q",
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

// Compile-time guard: the POST implementation continues
// to use the same SSH transport type as V2.
var _ provisioning.SSHClient
