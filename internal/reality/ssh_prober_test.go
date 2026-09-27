package reality

import (
	"context"
	"strings"
	"testing"
)

func TestSSHProberDecodesObservation(
	t *testing.T,
) {

	prober := SSHProber{
		Run: func(
			context.Context,
			string,
		) (string, error) {

			return `{
"reachable": true,
"tls_version": "TLSv1.3",
"cert_valid": true,
"http2": true,
"samples": 3,
"successes": 3,
"latency_ms": [
10,
11,
12
]
}`, nil
		},
	}

	observation, err := prober.Probe(
		context.Background(),

		Candidate{
			Target: "example.com",

			ServerName: "example.com",

			Port: 443,
		},

		3,
	)

	if err != nil {
		t.Fatal(err)
	}

	if !observation.CertValid {
		t.Fatal(
			"certificate should be valid",
		)
	}

	if observation.Successes != 3 {
		t.Fatalf(
			"unexpected successes: %d",
			observation.Successes,
		)
	}

	if !observation.HTTP2 {
		t.Fatal(
			"http2 should be detected",
		)
	}
}

func TestSSHProberDoesNotEmbedCandidatePlaintext(
	t *testing.T,
) {

	var command string

	prober := SSHProber{
		Run: func(
			_ context.Context,
			value string,
		) (string, error) {

			command = value

			return `{
"reachable": false,
"tls_version": "",
"cert_valid": false,
"http2": false,
"samples": 1,
"successes": 0,
"latency_ms": []
}`, nil
		},
	}

	_, _ = prober.Probe(
		context.Background(),

		Candidate{
			Target: "example.com",

			ServerName: "example.com",

			Port: 443,
		},

		1,
	)

	if strings.Contains(
		command,
		"example.com",
	) {
		t.Fatal(
			"candidate leaked into shell command",
		)
	}
}
