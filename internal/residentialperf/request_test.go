package residentialperf

import (
	"encoding/json"
	"os"
	"testing"
)

func TestRequestLegacyDeployedGoldenHashes(t *testing.T) {
	b, err := os.ReadFile("testdata/request-legacy-fc76e6a.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Request Request `json:"request"`
		Hash    string  `json:"hash"`
	}
	if err = json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		got, err := RequestHash(c.Request)
		if err != nil || got != c.Hash {
			t.Fatalf("legacy canonical digest changed for %s: %s %v", c.Request.Action, got, err)
		}
	}
}
func TestAdmissionTransportConsistencyMatrix(t *testing.T) {
	for _, c := range []struct {
		o    AdmissionObservation
		want bool
	}{
		{AdmissionObservation{Outcome: "ok", HTTPStatus: 200, CurlCode: 0}, true},
		{AdmissionObservation{Outcome: "ok", HTTPStatus: 200, CurlCode: 35}, false},
		{AdmissionObservation{Outcome: "ok", HTTPStatus: 204, CurlCode: 0}, false},
		{AdmissionObservation{Outcome: "timeout", HTTPStatus: 0, CurlCode: 28}, true},
		{AdmissionObservation{Outcome: "timeout", HTTPStatus: 0, CurlCode: 0}, false},
		{AdmissionObservation{Outcome: "tls", HTTPStatus: 0, CurlCode: 35}, true},
		{AdmissionObservation{Outcome: "tls", HTTPStatus: 200, CurlCode: 35}, false},
		{AdmissionObservation{Outcome: "http", HTTPStatus: 403, CurlCode: 0}, true},
		{AdmissionObservation{Outcome: "http", HTTPStatus: 200, CurlCode: 0}, false},
		{AdmissionObservation{Outcome: "http", HTTPStatus: 403, CurlCode: 28}, false},
		{AdmissionObservation{Outcome: "local_proxy_unavailable", HTTPStatus: 0, CurlCode: 7}, true},
		{AdmissionObservation{Outcome: "auth", HTTPStatus: 0, CurlCode: 67}, true},
		{AdmissionObservation{Outcome: "transport", HTTPStatus: 0, CurlCode: 56}, true},
		{AdmissionObservation{Outcome: "transport", HTTPStatus: 0, CurlCode: 0}, false},
		{AdmissionObservation{Outcome: "unknown", HTTPStatus: 0, CurlCode: 56}, false},
	} {
		if consistentObservation(c.o, 200) != c.want {
			t.Fatalf("inconsistent result: %+v", c)
		}
	}
}
