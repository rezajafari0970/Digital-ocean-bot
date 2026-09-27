package sanaei

import (
	"context"
	"testing"
)

type fakeExecutor struct {
	resp SessionResponse
	err  error
}

func (f fakeExecutor) Do(context.Context, SessionRequest) (SessionResponse, error) {
	return f.resp, f.err
}
func TestSessionExecutorContract(t *testing.T) {
	var e SessionExecutor = fakeExecutor{resp: SessionResponse{StatusCode: 200}}
	r, err := e.Do(context.Background(), SessionRequest{Method: "GET", Path: "panel/api/server/status"})
	if err != nil || r.StatusCode != 200 {
		t.Fatalf("response=%+v err=%v", r, err)
	}
}
