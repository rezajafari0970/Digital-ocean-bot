package reality

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrProbeFailed = errors.New("reality tls probe failed")

type probeWire struct {
	Reachable bool `json:"reachable"`

	TLSVersion string `json:"tls_version"`

	CertValid bool `json:"cert_valid"`

	HTTP2 bool `json:"http2"`

	Samples int `json:"samples"`

	Successes int `json:"successes"`

	LatencyMS []int64 `json:"latency_ms"`
}

func (p SSHProber) Probe(
	ctx context.Context,
	c Candidate,
	samples int,
) (Observation, error) {

	if p.Run == nil ||
		samples < 1 ||
		samples > 10 {

		return Observation{},
			ErrInvalidCandidate
	}

	if err := ValidateCandidate(c); err != nil {
		return Observation{}, err
	}

	payload, err := json.Marshal(
		map[string]any{
			"target": c.Target,

			"server_name": c.ServerName,

			"port": c.Port,

			"samples": samples,
		},
	)

	if err != nil {
		return Observation{},
			ErrProbeFailed
	}

	encoded :=
		base64.StdEncoding.EncodeToString(
			payload,
		)

	command := fmt.Sprintf(
		`
PROBE_B64='%s' python3 - <<'PY'
import base64
import json
import os
import socket
import ssl
import time

query=json.loads(
    base64.b64decode(
        os.environ["PROBE_B64"]
    )
)

result={
    "reachable": False,
    "tls_version": "",
    "cert_valid": False,
    "http2": False,
    "samples": query["samples"],
    "successes": 0,
    "latency_ms": [],
}

context=ssl.create_default_context()

context.minimum_version=(
    ssl.TLSVersion.TLSv1_3
)

context.set_alpn_protocols(
    [
        "h2",
        "http/1.1",
    ]
)

for _ in range(query["samples"]):
    started=time.monotonic()

    try:
        with socket.create_connection(
            (
                query["target"],
                query["port"],
            ),
            timeout=5,
        ) as raw:

            with context.wrap_socket(
                raw,
                server_hostname=query[
                    "server_name"
                ],
            ) as tls:

                elapsed=round(
                    (
                        time.monotonic()
                        -
                        started
                    )
                    *
                    1000
                )

                result["reachable"]=True

                result["cert_valid"]=True

                result["tls_version"]=(
                    tls.version()
                    or ""
                )

                if (
                    tls.selected_alpn_protocol()
                    ==
                    "h2"
                ):
                    result["http2"]=True

                result["successes"] += 1

                result[
                    "latency_ms"
                ].append(
                    elapsed
                )

    except Exception:
        pass

print(
    json.dumps(
        result,
        separators=(",",":"),
    )
)
PY
`,
		encoded,
	)

	output, err := p.Run(
		ctx,
		command,
	)

	if err != nil {
		return Observation{},
			ErrProbeFailed
	}

	var wire probeWire

	if err := json.Unmarshal(
		[]byte(output),
		&wire,
	); err != nil {
		return Observation{}, ErrProbeFailed
	}
	if wire.Samples != samples || wire.Successes < 0 || wire.Successes > samples || len(wire.LatencyMS) != wire.Successes {
		return Observation{}, ErrProbeFailed
	}

	return Observation{
		Candidate: c,

		Reachable: wire.Reachable,

		TLSVersion: wire.TLSVersion,

		CertValid: wire.CertValid,

		HTTP2: wire.HTTP2,

		Samples: wire.Samples,

		Successes: wire.Successes,

		LatencyMS: wire.LatencyMS,

		ObservedAt: time.Now().UTC(),
	}, nil
}
