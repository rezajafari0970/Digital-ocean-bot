package sanaei

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
			fmt.Errorf("%w: http=%d msg=%q", ErrMutationRejected, response.StatusCode, envelope.Msg)
	}

	return envelope.Obj, nil
}

func DeleteInbound(
	ctx context.Context,
	exec SessionExecutor,
	remoteID int64,
) error {
	if exec == nil || remoteID <= 0 {
		return ErrMutationRequest
	}
	response, err := exec.Do(ctx, SessionRequest{
		Method: http.MethodPost,
		Path:   fmt.Sprintf("panel/api/inbounds/del/%d", remoteID),
	})
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ErrMutationRequest
	}
	var envelope mutationEnvelope
	if json.Unmarshal(response.Body, &envelope) != nil {
		return ErrMutationRequest
	}
	if !envelope.Success {
		return ErrMutationRejected
	}
	return nil
}

func UpdateInbound(
	ctx context.Context,
	exec SessionExecutor,
	remoteID int64,
	payload realityconfig.Payload,
) (json.RawMessage, error) {
	if exec == nil || remoteID <= 0 {
		return nil, ErrMutationRequest
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	response, err := exec.Do(ctx, SessionRequest{
		Method: http.MethodPost,
		Path:   fmt.Sprintf("panel/api/inbounds/update/%d", remoteID),
		Body:   body,
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, ErrMutationRequest
	}
	var envelope mutationEnvelope
	if json.Unmarshal(response.Body, &envelope) != nil {
		return nil, ErrMutationRequest
	}
	if !envelope.Success {
		return nil, ErrMutationRejected
	}
	return envelope.Obj, nil
}

// UpdateInboundRaw updates an inbound through the official
// Sanaei API while preserving an arbitrary full JSON payload.
//
// This is used for API-only operations that are not limited
// to the Reality builder schema, such as editing clients
// inside settings.clients.
func UpdateInboundRaw(
	ctx context.Context,
	exec SessionExecutor,
	remoteID int64,
	payload any,
) (
	json.RawMessage,
	error,
) {

	if exec == nil ||
		remoteID <= 0 ||
		payload == nil {

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

				Path: fmt.Sprintf(
					"panel/api/inbounds/update/%d",
					remoteID,
				),

				Body: body,

				ContentType: "application/json",
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

	if err :=
		json.Unmarshal(
			response.Body,
			&envelope,
		); err != nil {

		return nil,
			ErrMutationRequest
	}

	if !envelope.Success {

		return nil,
			fmt.Errorf(
				"%w: http=%d msg=%q",
				ErrMutationRejected,
				response.StatusCode,
				envelope.Msg,
			)
	}

	return envelope.Obj, nil
}
