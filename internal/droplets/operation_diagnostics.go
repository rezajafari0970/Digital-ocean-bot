package droplets

import (
	"context"
	"errors"
	"fmt"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

type operationStage uint8

const (
	stagePreCreate operationStage = iota
	stageCreate
	stageCreateRejected
	stageCreateAmbiguous
	stageCreateEgress
	stageDeletePreEgress
	stageDelete
	stageDeleteEgress
)

func operationDiagnostic(stage operationStage, err error) (string, string) {
	label := "operation"
	switch stage {
	case stagePreCreate:
		label = "pre_create"
	case stageCreate:
		label = "create_server"
	case stageCreateRejected:
		label = "create_rejected"
	case stageCreateAmbiguous:
		label = "create_ambiguous"
	case stageCreateEgress:
		label = "create_egress"
	case stageDeletePreEgress:
		label = "delete_pre_egress"
	case stageDelete:
		label = "delete_server"
	case stageDeleteEgress:
		label = "delete_egress"
	}
	// Never format err, its provider message/code, or its wrapped cause.
	code := "unknown"
	switch c := providers.Class(err); c {
	case providers.ErrorUnknown, providers.ErrorBilling, providers.ErrorAuthentication,
		providers.ErrorPermissionDenied, providers.ErrorAccountLocked, providers.ErrorRateLimited,
		providers.ErrorCapacity, providers.ErrorRegionCapacity, providers.ErrorImageUnavailable,
		providers.ErrorNotFound, providers.ErrorInvalidRequest, providers.ErrorTransport,
		providers.ErrorUnavailable, providers.ErrorAmbiguousOutcome:
		code = string(c)
	}
	switch {
	case errors.Is(err, ErrMutationBlocked):
		code = "mutation_blocked"
	case errors.Is(err, ErrOutcomeStillUnknown):
		code = "ambiguous_outcome"
	case errors.Is(err, context.DeadlineExceeded):
		code = "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		code = "cancelled"
	}
	message := label + ": " + code
	var pe *providers.Error
	if errors.As(err, &pe) && pe != nil && pe.StatusCode >= 100 && pe.StatusCode <= 599 {
		message += fmt.Sprintf(" (HTTP %d)", pe.StatusCode)
	}
	return code, message
}

func (e Executor) recordOperationError(ctx context.Context, op jobs.Operation, stage operationStage, cause error) (jobs.Operation, error) {
	op.ErrorCode, op.ErrorMessage = operationDiagnostic(stage, cause)
	if err := e.Operations.Update(ctx, &op); err != nil {
		// A failed journal write cannot authorize provider-specific fallback.
		// Return the persistence failure, not a provider capacity classification.
		return op, fmt.Errorf("persist operation outcome: %w", err)
	}
	return op, cause
}
