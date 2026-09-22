package jobs

import (
	"errors"
	"time"
)

type OperationState string

const (
	OperationPlanned   OperationState = "planned"
	OperationRunning   OperationState = "running"
	OperationVerifying OperationState = "verifying"
	OperationSucceeded OperationState = "succeeded"
	OperationFailed    OperationState = "failed"
	OperationUnknown   OperationState = "unknown"
)

var ErrOperationTenantMismatch = errors.New("operation tenant mismatch")

type Operation struct {
	ID               string
	AccountID        string
	Kind             string
	IdempotencyKey   string
	State            OperationState
	ProviderActionID string
	ResourceID       string
	Attempt          int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (o Operation) Authorize(accountID string) error {
	if o.AccountID == "" || o.AccountID != accountID {
		return ErrOperationTenantMismatch
	}
	return nil
}
