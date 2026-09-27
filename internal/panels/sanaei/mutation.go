package sanaei

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
)

var (
	ErrMutationRequest = errors.New(
		"panel mutation request failed",
	)

	ErrMutationRejected = errors.New(
		"panel mutation rejected",
	)
)

type mutationEnvelope struct {
	Success bool `json:"success"`

	Msg string `json:"msg"`

	Obj json.RawMessage `json:"obj"`
}

func AddInbound(
	ctx context.Context,
	exec SessionExecutor,
	payload realityconfig.Payload,
) (json.RawMessage, error) {

	if exec == nil {
		return nil,
			ErrMutationRequest
	}

	body, err :=
		json.Marshal(
			payload,
		)

	if err != nil {
		return nil, err
	}

	response, err :=
		exec.Do(
			ctx,

			SessionRequest{
				Method: http.MethodPost,

				Path: "panel/api/inbounds/add",

				Body: body,
			},
		)

	if err != nil {
		return nil, err
	}

	if response.StatusCode < 200 ||
		response.StatusCode >= 300 {

		return nil,
			ErrMutationRequest
	}

	var envelope mutationEnvelope

	if err = json.Unmarshal(
		response.Body,
		&envelope,
	); err != nil {

		return nil,
			ErrMutationRequest
	}

	if !envelope.Success {
		return nil,
			ErrMutationRejected
	}

	return envelope.Obj, nil
}
