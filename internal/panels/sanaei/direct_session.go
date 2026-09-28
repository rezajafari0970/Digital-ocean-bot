package sanaei

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

type DirectSessionExecutor struct{ Client *APIClient }

func (d DirectSessionExecutor) Do(ctx context.Context, x SessionRequest) (SessionResponse, error) {
	if d.Client == nil || d.Client.HTTP == nil {
		return SessionResponse{}, ErrSessionRequest
	}
	var body io.Reader
	if len(x.Body) > 0 {
		body = strings.NewReader(string(x.Body))
	}
	req, err := http.NewRequestWithContext(ctx, x.Method, d.Client.BaseURL+"/"+strings.TrimLeft(x.Path, "/"), body)
	if err != nil {
		return SessionResponse{}, ErrSessionRequest
	}
	if x.ContentType != "" {
		req.Header.Set("Content-Type", x.ContentType)
	}
	if d.Client.CSRF != "" {
		req.Header.Set("X-CSRF-Token", d.Client.CSRF)
	}
	client := d.Client.HTTP
	if x.TimeoutSeconds > 0 {
		clone := *client
		clone.Timeout = time.Duration(x.TimeoutSeconds) * time.Second
		client = &clone
	}
	resp, err := client.Do(req)
	if err != nil {
		return SessionResponse{}, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return SessionResponse{}, err
	}
	return SessionResponse{StatusCode: resp.StatusCode, Body: b}, nil
}
