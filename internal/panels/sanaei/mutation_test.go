package sanaei

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
)

type captureMutationExecutor struct {
	Request SessionRequest

	Response SessionResponse
}

func (e *captureMutationExecutor) Do(
	_ context.Context,
	request SessionRequest,
) (SessionResponse, error) {

	e.Request = request

	return e.Response, nil
}

func TestAddInboundUsesRestrictedEndpoint(
	t *testing.T,
) {

	executor :=
		&captureMutationExecutor{
			Response: SessionResponse{
				StatusCode: 200,

				Body: []byte(
					`{
"success": true,
"obj": {
"id": 7
}
}`,
				),
			},
		}

	payload :=
		realityconfig.Payload{
			Enable: true,

			Remark: "dob:test",

			Port: 443,

			Protocol: "vless",

			Settings: map[string]any{},

			StreamSettings: map[string]any{},

			Sniffing: map[string]any{},
		}

	object, err :=
		AddInbound(
			context.Background(),
			executor,
			payload,
		)

	if err != nil {
		t.Fatal(err)
	}

	if executor.Request.Method !=
		"POST" {

		t.Fatalf(
			"unexpected method: %s",
			executor.Request.Method,
		)
	}

	if executor.Request.Path !=
		"panel/api/inbounds/add" {

		t.Fatalf(
			"unexpected path: %s",
			executor.Request.Path,
		)
	}

	var decoded map[string]any

	if err = json.Unmarshal(
		object,
		&decoded,
	); err != nil {

		t.Fatal(err)
	}

	if decoded["id"].(float64) != 7 {
		t.Fatalf(
			"unexpected object: %s",
			object,
		)
	}
}

func TestAddInboundRejectsSuccessFalse(
	t *testing.T,
) {

	executor :=
		&captureMutationExecutor{
			Response: SessionResponse{
				StatusCode: 200,

				Body: []byte(
					`{
"success": false,
"msg": "rejected"
}`,
				),
			},
		}

	_, err :=
		AddInbound(
			context.Background(),
			executor,
			realityconfig.Payload{},
		)

	if !errors.Is(
		err,
		ErrMutationRejected,
	) {
		t.Fatalf(
			"unexpected error: %v",
			err,
		)
	}
}

func TestDeleteClientByEmailUsesV3Route(t *testing.T) {
	e := &captureMutationExecutor{Response: SessionResponse{StatusCode: 200, Body: []byte(`{"success":true}`)}}
	if err := DeleteClientByEmailSession(context.Background(), e, "user+one@example.com"); err != nil {
		t.Fatal(err)
	}
	if e.Request.Path != "panel/api/clients/del/user+one@example.com" {
		t.Fatalf("path=%q", e.Request.Path)
	}
}

func TestDeleteClientByEmail404IsRouteUnsupported(t *testing.T) {
	e := &captureMutationExecutor{Response: SessionResponse{StatusCode: 404}}
	err := DeleteClientByEmailSession(context.Background(), e, "u@example.com")
	if !errors.Is(err, ErrDeleteClientRouteUnsupported) {
		t.Fatalf("err=%v", err)
	}
}

type scriptedMutationExecutor struct {
	responses []SessionResponse
	errs      []error
	requests  []SessionRequest
}

func (e *scriptedMutationExecutor) Do(_ context.Context, r SessionRequest) (SessionResponse, error) {
	e.requests = append(e.requests, r)
	i := len(e.requests) - 1
	var resp SessionResponse
	if i < len(e.responses) {
		resp = e.responses[i]
	}
	var err error
	if i < len(e.errs) {
		err = e.errs[i]
	}
	return resp, err
}

func TestDeleteClientCompatiblePrefersV3(t *testing.T) {
	e := &scriptedMutationExecutor{responses: []SessionResponse{{StatusCode: 200, Body: []byte(`{"success":true}`)}}}
	if err := DeleteClientCompatibleSession(context.Background(), e, 1, "uuid", "u@example.com"); err != nil {
		t.Fatal(err)
	}
	if len(e.requests) != 1 || e.requests[0].Path != "panel/api/clients/del/u@example.com" {
		t.Fatalf("requests=%+v", e.requests)
	}
}
func TestDeleteClientCompatibleFallsBackOnlyOnUnsupportedRoute(t *testing.T) {
	e := &scriptedMutationExecutor{responses: []SessionResponse{{StatusCode: 404}, {StatusCode: 200, Body: []byte(`{"success":true}`)}}}
	if err := DeleteClientCompatibleSession(context.Background(), e, 1, "uuid", "u@example.com"); err != nil {
		t.Fatal(err)
	}
	if len(e.requests) != 2 || e.requests[1].Path != "panel/api/inbounds/1/delClient/uuid" {
		t.Fatalf("requests=%+v", e.requests)
	}
}
func TestDeleteClientCompatibleDoesNotFallbackOnServerError(t *testing.T) {
	e := &scriptedMutationExecutor{responses: []SessionResponse{{StatusCode: 500, Body: []byte("boom")}}}
	if err := DeleteClientCompatibleSession(context.Background(), e, 1, "uuid", "u@example.com"); err == nil {
		t.Fatal("expected error")
	}
	if len(e.requests) != 1 {
		t.Fatalf("requests=%d", len(e.requests))
	}
}
func TestDeleteClientCompatibleDoesNotFallbackOnTransportError(t *testing.T) {
	boom := errors.New("transport")
	e := &scriptedMutationExecutor{errs: []error{boom}}
	if err := DeleteClientCompatibleSession(context.Background(), e, 1, "uuid", "u@example.com"); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	if len(e.requests) != 1 {
		t.Fatalf("requests=%d", len(e.requests))
	}
}

func TestUpdateClientByEmailUsesV3FullReplacementRoute(t *testing.T) {
	e := &captureMutationExecutor{Response: SessionResponse{StatusCode: 200, Body: []byte(`{"success":true}`)}}
	payload := map[string]any{"id": "u1", "email": "u@example.com", "totalGB": int64(123), "custom": "keep"}
	if err := UpdateClientByEmailSession(context.Background(), e, "u@example.com", payload); err != nil {
		t.Fatal(err)
	}
	if e.Request.Path != "panel/api/clients/update/u@example.com" || e.Request.ContentType != "application/json" {
		t.Fatalf("request=%+v", e.Request)
	}
	var got map[string]any
	if err := json.Unmarshal(e.Request.Body, &got); err != nil {
		t.Fatal(err)
	}
	if got["custom"] != "keep" {
		t.Fatalf("payload=%v", got)
	}
}

func TestAddClientCompatiblePrefersV3(t *testing.T) {
	e := &scriptedMutationExecutor{responses: []SessionResponse{{StatusCode: 200, Body: []byte(`{"success":true}`)}}}
	c := Client{ID: "u1", Email: "u@example.com", Enable: true, LimitHWID: 2}
	if err := AddClientCompatibleSession(context.Background(), e, 7, c); err != nil {
		t.Fatal(err)
	}
	if len(e.requests) != 1 || e.requests[0].Path != "panel/api/clients/add" || e.requests[0].ContentType != "application/json" {
		t.Fatalf("requests=%+v", e.requests)
	}
	var body map[string]any
	if err := json.Unmarshal(e.requests[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	ids := body["inboundIds"].([]any)
	if len(ids) != 1 || int(ids[0].(float64)) != 7 {
		t.Fatalf("body=%v", body)
	}
}
func TestAddClientCompatibleFallsBackOnlyOnUnsupportedRoute(t *testing.T) {
	e := &scriptedMutationExecutor{responses: []SessionResponse{{StatusCode: 404}, {StatusCode: 200, Body: []byte(`{"success":true}`)}}}
	if err := AddClientCompatibleSession(context.Background(), e, 7, Client{ID: "u1", Email: "u@example.com"}); err != nil {
		t.Fatal(err)
	}
	if len(e.requests) != 2 || e.requests[1].Path != "panel/api/inbounds/addClient" {
		t.Fatalf("requests=%+v", e.requests)
	}
}
