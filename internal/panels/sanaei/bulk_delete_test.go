package sanaei

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type bulkDeleteStub struct {
	calls    int
	response SessionResponse
	err      error
	request  SessionRequest
}

func (s *bulkDeleteStub) Do(_ context.Context, r SessionRequest) (SessionResponse, error) {
	s.calls++
	s.request = r
	return s.response, s.err
}
func TestBulkDeleteNeverRetriesOrFallsBack(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		err    error
	}{
		{200, `{"success":true,"obj":{"deleted":1,"skipped":[{"email":"b","reason":"not found"}]}}`, nil},
		{404, `{}`, nil}, {500, `{}`, nil}, {200, `broken`, nil}, {200, `{"success":true}`, nil}, {200, `{"success":true,"obj":{"deleted":99}}`, nil}, {0, "", errors.New("lost response")},
	} {
		s := &bulkDeleteStub{response: SessionResponse{StatusCode: tc.status, Body: []byte(tc.body)}, err: tc.err}
		result, err := BulkDeleteClientsSession(context.Background(), s, []string{"a", "b"})
		good := tc.status == 200 && tc.body == `{"success":true,"obj":{"deleted":1,"skipped":[{"email":"b","reason":"not found"}]}}`
		if good && (err != nil || result.Deleted != 1 || len(result.Skipped) != 1) {
			t.Fatal(result, err)
		}
		if !good && err == nil {
			t.Fatal("bad response accepted")
		}
		if s.calls != 1 || s.request.Path != "panel/api/clients/bulkDel" {
			t.Fatal("retry/fallback", s.calls, s.request.Path)
		}
		var p struct {
			Emails      []string `json:"emails"`
			KeepTraffic bool     `json:"keepTraffic"`
		}
		if json.Unmarshal(s.request.Body, &p) != nil || len(p.Emails) != 2 || p.KeepTraffic {
			t.Fatal("bad payload")
		}
	}
}
