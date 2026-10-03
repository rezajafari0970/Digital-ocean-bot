package sanaei

import (
	"context"
	"testing"
)

func TestRawInboundListRejectsAmbiguousEmpty(t *testing.T) {
	for _, body := range []string{`{"success":true}`, `{"success":true,"obj":null}`, `{"success":true,"obj":{}}`} {
		stub := &bulkDeleteStub{response: SessionResponse{StatusCode: 200, Body: []byte(body)}}
		if _, err := ReadRawInboundList(context.Background(), stub); err == nil {
			t.Fatal("ambiguous absence accepted")
		}
	}
	stub := &bulkDeleteStub{response: SessionResponse{StatusCode: 200, Body: []byte(`{"success":true,"obj":[]}`)}}
	if cs, err := ReadRawInboundList(context.Background(), stub); err != nil || len(cs) != 0 {
		t.Fatal(cs, err)
	}
}
