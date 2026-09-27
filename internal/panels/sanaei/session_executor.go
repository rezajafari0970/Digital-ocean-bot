package sanaei

import (
	"context"
	"errors"
	"net/http"
)

var ErrSessionRequest = errors.New("sanaei session request failed")

type SessionRequest struct {
	Method string
	Path   string
	Body   []byte
}

type SessionResponse struct {
	StatusCode int
	Body       []byte
}

type SessionExecutor interface {
	Do(context.Context, SessionRequest) (SessionResponse, error)
}

type ExecutorAuth struct{ Executor SessionExecutor }

func (e ExecutorAuth) Authorize(context.Context, *http.Request) error {
	if e.Executor == nil {
		return ErrSessionRequest
	}
	return nil
}
