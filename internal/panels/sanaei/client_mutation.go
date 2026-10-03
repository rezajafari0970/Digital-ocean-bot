package sanaei

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

var ErrAddClientUnsupported = errors.New("sanaei addClient unsupported")

func AddClientsSession(ctx context.Context, exec SessionExecutor, inboundID int, clients []Client) error {
	if exec == nil || inboundID <= 0 || len(clients) == 0 {
		return ErrMutationRequest
	}
	settings, err := json.Marshal(map[string]any{"clients": clients})
	if err != nil {
		return err
	}
	form := url.Values{"id": {fmt.Sprintf("%d", inboundID)}, "settings": {string(settings)}}
	resp, err := exec.Do(ctx, SessionRequest{
		Method:         http.MethodPost,
		Path:           "panel/api/inbounds/addClient",
		Body:           []byte(form.Encode()),
		ContentType:    "application/x-www-form-urlencoded",
		TimeoutSeconds: 12,
	})
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return fmt.Errorf("%w: http=%d", ErrAddClientUnsupported, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := resp.Body
		if len(body) > 512 {
			body = body[:512]
		}
		return fmt.Errorf("%w: addClient http=%d body=%q", ErrMutationRequest, resp.StatusCode, string(body))
	}
	var envelope mutationEnvelope
	if json.Unmarshal(resp.Body, &envelope) != nil {
		body := resp.Body
		if len(body) > 512 {
			body = body[:512]
		}
		return fmt.Errorf("%w: addClient invalid json body=%q", ErrMutationRequest, string(body))
	}
	if !envelope.Success {
		return fmt.Errorf("%w: http=%d msg=%q", ErrMutationRejected, resp.StatusCode, envelope.Msg)
	}
	return nil
}

func DeleteClientSession(ctx context.Context, exec SessionExecutor, inboundID int, clientID string) error {
	if exec == nil || inboundID <= 0 || clientID == "" {
		return ErrMutationRequest
	}
	resp, err := exec.Do(ctx, SessionRequest{
		Method:         http.MethodPost,
		Path:           fmt.Sprintf("panel/api/inbounds/%d/delClient/%s", inboundID, url.PathEscape(clientID)),
		TimeoutSeconds: 8,
	})
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := resp.Body
		if len(body) > 512 {
			body = body[:512]
		}
		return fmt.Errorf("%w: delClient http=%d body=%q", ErrMutationRequest, resp.StatusCode, string(body))
	}
	var envelope mutationEnvelope
	if json.Unmarshal(resp.Body, &envelope) != nil {
		body := resp.Body
		if len(body) > 512 {
			body = body[:512]
		}
		return fmt.Errorf("%w: delClient invalid json body=%q", ErrMutationRequest, string(body))
	}
	if !envelope.Success {
		return fmt.Errorf("%w: http=%d msg=%q", ErrMutationRejected, resp.StatusCode, envelope.Msg)
	}
	return nil
}
