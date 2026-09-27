package realityconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildRealityInbound(
	t *testing.T,
) {

	raw, err := Marshal(
		Input{
			Remark: "dob:reality:000001",

			Port: 443,

			UUID: "11111111-1111-4111-8111-111111111111",

			Email: "user1",

			Target: "www.example.com",

			ServerName: "www.example.com",

			PrivateKey: "private-key",

			ShortID: "a1b2c3d4",
		},
	)

	if err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any

	if err := json.Unmarshal(
		raw,
		&decoded,
	); err != nil {

		t.Fatal(err)
	}

	if decoded["protocol"] != "vless" {
		t.Fatalf(
			"unexpected payload: %s",
			raw,
		)
	}

	if !strings.Contains(
		string(raw),
		`"security":"reality"`,
	) {
		t.Fatalf(
			"reality security missing: %s",
			raw,
		)
	}

	if !strings.Contains(
		string(raw),
		`"network":"tcp"`,
	) {
		t.Fatalf(
			"tcp network missing: %s",
			raw,
		)
	}
}

func TestBuildRejectsUnsafeTarget(
	t *testing.T,
) {

	_, err := Build(
		Input{
			Remark: "x",

			Port: 443,

			UUID: "u",

			Target: "bad host",

			ServerName: "example.com",

			PrivateKey: "k",

			ShortID: "aa",
		},
	)

	if err == nil {
		t.Fatal(
			"expected invalid target",
		)
	}
}
