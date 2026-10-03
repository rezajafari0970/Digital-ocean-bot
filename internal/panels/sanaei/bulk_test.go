package sanaei

import (
	"context"
	"errors"
	"testing"
)

func TestBulkCreateParsesPartialAndRejectsMissingObject(t *testing.T) {
	e := &captureMutationExecutor{Response: SessionResponse{StatusCode: 200, Body: []byte(`{"success":true,"obj":{"created":1,"skipped":[{"email":"b","reason":"duplicate"}]}}`)}}
	out, err := BulkCreateClientsSession(context.Background(), e, 1, []Client{{ID: "a", Email: "a"}, {ID: "b", Email: "b"}})
	if err != nil || out.Created != 1 || len(out.Skipped) != 1 || out.Skipped[0].Email != "b" {
		t.Fatal(out, err)
	}
	e.Response.Body = []byte(`{"success":true}`)
	if _, err = BulkCreateClientsSession(context.Background(), e, 1, []Client{{ID: "a", Email: "a"}}); err == nil {
		t.Fatal("missing object accepted")
	}
}
func TestBulkCreateNoFallbackOnUnknownOutcome(t *testing.T) {
	for _, tc := range []struct {
		response SessionResponse
		err      error
	}{{SessionResponse{}, errors.New("lost response")}, {SessionResponse{StatusCode: 500}, nil}, {SessionResponse{StatusCode: 200, Body: []byte("truncated")}, nil}} {
		e := &scriptedMutationExecutor{responses: []SessionResponse{tc.response}, errs: []error{tc.err}}
		if _, err := BulkCreateCompatibleSession(context.Background(), e, 1, []Client{{ID: "a", Email: "a"}}); err == nil {
			t.Fatal("expected failure")
		}
		if len(e.requests) != 1 {
			t.Fatal("blind fallback", e.requests)
		}
	}
}
