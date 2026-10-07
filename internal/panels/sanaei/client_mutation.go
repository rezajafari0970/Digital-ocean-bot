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
		return fmt.Errorf("%w: addClient http=%d body=%q", panelResponseError(ErrMutationRequest), resp.StatusCode, string(body))
	}
	var envelope mutationEnvelope
	if json.Unmarshal(resp.Body, &envelope) != nil {
		body := resp.Body
		if len(body) > 512 {
			body = body[:512]
		}
		return fmt.Errorf("%w: addClient invalid json body=%q", panelResponseError(ErrMutationRequest), string(body))
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
		return fmt.Errorf("%w: delClient http=%d body=%q", panelResponseError(ErrMutationRequest), resp.StatusCode, string(body))
	}
	var envelope mutationEnvelope
	if json.Unmarshal(resp.Body, &envelope) != nil {
		body := resp.Body
		if len(body) > 512 {
			body = body[:512]
		}
		return fmt.Errorf("%w: delClient invalid json body=%q", panelResponseError(ErrMutationRequest), string(body))
	}
	if !envelope.Success {
		return fmt.Errorf("%w: http=%d msg=%q", ErrMutationRejected, resp.StatusCode, envelope.Msg)
	}
	return nil
}

var ErrDeleteClientRouteUnsupported = errors.New("sanaei delete client route unsupported")

func DeleteClientByEmailSession(ctx context.Context, exec SessionExecutor, email string) error {
	if exec == nil || email == "" {
		return ErrMutationRequest
	}
	resp, err := exec.Do(ctx, SessionRequest{
		Method:         http.MethodPost,
		Path:           "panel/api/clients/del/" + url.PathEscape(email),
		TimeoutSeconds: 8,
	})
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return fmt.Errorf("%w: http=%d", ErrDeleteClientRouteUnsupported, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := resp.Body
		if len(body) > 512 {
			body = body[:512]
		}
		return fmt.Errorf("%w: clients/del http=%d body=%q", panelResponseError(ErrMutationRequest), resp.StatusCode, string(body))
	}
	var envelope mutationEnvelope
	if json.Unmarshal(resp.Body, &envelope) != nil {
		body := resp.Body
		if len(body) > 512 {
			body = body[:512]
		}
		return fmt.Errorf("%w: clients/del invalid json body=%q", panelResponseError(ErrMutationRequest), string(body))
	}
	if !envelope.Success {
		return fmt.Errorf("%w: http=%d msg=%q", ErrMutationRejected, resp.StatusCode, envelope.Msg)
	}
	return nil
}

func DeleteClientCompatibleSession(ctx context.Context, exec SessionExecutor, inboundID int, clientID, email string) error {
	if email == "" {
		return ErrMutationRequest
	}
	err := DeleteClientByEmailSession(ctx, exec, email)
	if !errors.Is(err, ErrDeleteClientRouteUnsupported) {
		return err
	}
	return DeleteClientSession(ctx, exec, inboundID, clientID)
}

func UpdateClientByEmailSession(ctx context.Context, exec SessionExecutor, currentEmail string, client map[string]any) error {
	if exec == nil || currentEmail == "" || len(client) == 0 {
		return ErrMutationRequest
	}
	body, err := json.Marshal(client)
	if err != nil {
		return err
	}
	resp, err := exec.Do(ctx, SessionRequest{Method: http.MethodPost, Path: "panel/api/clients/update/" + url.PathEscape(currentEmail), Body: body, ContentType: "application/json", TimeoutSeconds: 8})
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b := resp.Body
		if len(b) > 512 {
			b = b[:512]
		}
		return fmt.Errorf("%w: clients/update http=%d body=%q", panelResponseError(ErrMutationRequest), resp.StatusCode, string(b))
	}
	var envelope mutationEnvelope
	if json.Unmarshal(resp.Body, &envelope) != nil {
		b := resp.Body
		if len(b) > 512 {
			b = b[:512]
		}
		return fmt.Errorf("%w: clients/update invalid json body=%q", panelResponseError(ErrMutationRequest), string(b))
	}
	if !envelope.Success {
		return fmt.Errorf("%w: http=%d msg=%q", ErrMutationRejected, resp.StatusCode, envelope.Msg)
	}
	return nil
}

func AddClientV3Session(ctx context.Context, exec SessionExecutor, inboundID int, client Client) error {
	if exec == nil || inboundID <= 0 || client.Email == "" {
		return ErrMutationRequest
	}
	body, err := json.Marshal(map[string]any{"client": client, "inboundIds": []int{inboundID}})
	if err != nil {
		return err
	}
	resp, err := exec.Do(ctx, SessionRequest{Method: http.MethodPost, Path: "panel/api/clients/add", Body: body, ContentType: "application/json", TimeoutSeconds: 12})
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return fmt.Errorf("%w: http=%d", ErrAddClientUnsupported, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b := resp.Body
		if len(b) > 512 {
			b = b[:512]
		}
		return fmt.Errorf("%w: clients/add http=%d body=%q", panelResponseError(ErrMutationRequest), resp.StatusCode, string(b))
	}
	var envelope mutationEnvelope
	if json.Unmarshal(resp.Body, &envelope) != nil {
		return panelResponseError(ErrMutationRequest)
	}
	if !envelope.Success {
		return fmt.Errorf("%w: http=%d msg=%q", ErrMutationRejected, resp.StatusCode, envelope.Msg)
	}
	return nil
}
func AddClientCompatibleSession(ctx context.Context, exec SessionExecutor, inboundID int, client Client) error {
	err := AddClientV3Session(ctx, exec, inboundID, client)
	if !errors.Is(err, ErrAddClientUnsupported) {
		return err
	}
	return AddClientsSession(ctx, exec, inboundID, []Client{client})
}

func GetClientByEmailSession(ctx context.Context, exec SessionExecutor, email string) (map[string]any, error) {
	if exec == nil || email == "" {
		return nil, ErrMutationRequest
	}
	resp, err := exec.Do(ctx, SessionRequest{Method: http.MethodGet, Path: "panel/api/clients/get/" + url.PathEscape(email), TimeoutSeconds: 8})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: clients/get http=%d", panelResponseError(ErrMutationRequest), resp.StatusCode)
	}
	var env struct {
		Success bool   `json:"success"`
		Msg     string `json:"msg"`
		Obj     struct {
			Client map[string]any `json:"client"`
		} `json:"obj"`
	}
	if json.Unmarshal(resp.Body, &env) != nil || !env.Success || len(env.Obj.Client) == 0 {
		return nil, fmt.Errorf("%w: clients/get msg=%q", ErrMutationRejected, env.Msg)
	}
	return env.Obj.Client, nil
}

type BulkCreateSkipped struct {
	Email  string `json:"email"`
	Reason string `json:"reason"`
}
type BulkCreateResult struct {
	Created int                 `json:"created"`
	Skipped []BulkCreateSkipped `json:"skipped"`
}

func BulkCreateClientsSession(ctx context.Context, exec SessionExecutor, inboundID int, clients []Client) (BulkCreateResult, error) {
	var out BulkCreateResult
	if exec == nil || inboundID <= 0 || len(clients) == 0 {
		return out, ErrMutationRequest
	}
	items := make([]map[string]any, 0, len(clients))
	for _, c := range clients {
		items = append(items, map[string]any{"client": c, "inboundIds": []int{inboundID}})
	}
	body, err := json.Marshal(items)
	if err != nil {
		return out, err
	}
	resp, err := exec.Do(ctx, SessionRequest{Method: http.MethodPost, Path: "panel/api/clients/bulkCreate", Body: body, ContentType: "application/json", TimeoutSeconds: 30})
	if err != nil {
		return out, err
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return out, fmt.Errorf("%w: bulkCreate http=%d", ErrAddClientUnsupported, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("%w: bulkCreate http=%d", panelResponseError(ErrMutationRequest), resp.StatusCode)
	}
	var envelope struct {
		Success bool              `json:"success"`
		Msg     string            `json:"msg"`
		Obj     *BulkCreateResult `json:"obj"`
	}
	if json.Unmarshal(resp.Body, &envelope) != nil {
		return out, panelResponseError(ErrMutationRequest)
	}
	if envelope.Obj == nil {
		return out, panelResponseError(ErrMutationRequest)
	}
	out = *envelope.Obj
	if out.Created < 0 || out.Created > len(clients) || len(out.Skipped) > len(clients) {
		return out, panelResponseError(ErrMutationRequest)
	}
	if !envelope.Success {
		return out, fmt.Errorf("%w: bulkCreate msg=%q", ErrMutationRejected, envelope.Msg)
	}
	return out, nil
}

func BulkCreateCompatibleSession(ctx context.Context, exec SessionExecutor, inboundID int, clients []Client) (BulkCreateResult, error) {
	out, err := BulkCreateClientsSession(ctx, exec, inboundID, clients)
	if !errors.Is(err, ErrAddClientUnsupported) {
		return out, err
	}
	if err = AddClientsSession(ctx, exec, inboundID, clients); err != nil {
		return out, err
	}
	out.Created = len(clients)
	return out, nil
}

// BulkDeleteResult is diagnostic only; fresh identity absence decides success.
type BulkDeleteResult struct {
	Deleted int                 `json:"deleted"`
	Skipped []BulkCreateSkipped `json:"skipped"`
}

func BulkDeleteClientsSession(ctx context.Context, exec SessionExecutor, emails []string) (BulkDeleteResult, error) {
	var out BulkDeleteResult
	if exec == nil || len(emails) == 0 || len(emails) > 100 {
		return out, ErrMutationRequest
	}
	seen := map[string]bool{}
	for _, email := range emails {
		if email == "" || seen[email] {
			return out, ErrMutationRequest
		}
		seen[email] = true
	}
	body, err := json.Marshal(map[string]any{"emails": emails, "keepTraffic": false})
	if err != nil {
		return out, err
	}
	resp, err := exec.Do(ctx, SessionRequest{Method: http.MethodPost, Path: "panel/api/clients/bulkDel", Body: body, ContentType: "application/json", TimeoutSeconds: 30})
	if err != nil {
		return out, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("%w: bulkDel http=%d", panelResponseError(ErrMutationRequest), resp.StatusCode)
	}
	var envelope struct {
		Success bool              `json:"success"`
		Obj     *BulkDeleteResult `json:"obj"`
	}
	if json.Unmarshal(resp.Body, &envelope) != nil || envelope.Obj == nil {
		return out, panelResponseError(ErrMutationRequest)
	}
	out = *envelope.Obj
	if out.Deleted < 0 || out.Deleted > len(emails) || len(out.Skipped) > len(emails) {
		return out, panelResponseError(ErrMutationRequest)
	}
	if !envelope.Success {
		return out, ErrMutationRejected
	}
	return out, nil
}
