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
