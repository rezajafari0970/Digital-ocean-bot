package providers

import (
	"context"
	"errors"
	"math/big"
	"time"
)

type BillingReader interface {
	Billing(context.Context) (Billing, error)
}
type Billing struct {
	Currency           string    `json:"currency"`
	Balance            string    `json:"balance"`
	PendingCharges     string    `json:"pending_charges"`
	GeneratedAt        time.Time `json:"generated_at"`
	Invoices           []Invoice `json:"invoices"`
	InvoiceStatusKnown bool      `json:"invoice_status_known"`
}
type Invoice struct {
	ID     string `json:"id"`
	Amount string `json:"amount"`
	Date   string `json:"date"`
	Status string `json:"status"`
}

func ValidateMoney(value string) error {
	if len(value) == 0 || len(value) > 40 {
		return errors.New("missing or invalid billing amount")
	}
	for i, c := range value {
		if (c < '0' || c > '9') && c != '.' && !(c == '-' && i == 0) {
			return errors.New("invalid billing amount")
		}
	}
	if _, ok := new(big.Rat).SetString(value); !ok {
		return errors.New("invalid billing amount")
	}
	return nil
}
