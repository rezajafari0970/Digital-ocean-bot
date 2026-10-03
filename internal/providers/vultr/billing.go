package vultr

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"time"
)

func (d *Driver) Billing(ctx context.Context) (providers.Billing, error) {
	var account struct {
		Account struct {
			Balance json.Number `json:"balance"`
			Pending json.Number `json:"pending_charges"`
		} `json:"account"`
	}
	out := providers.Billing{Currency: "USD", Invoices: []providers.Invoice{}, InvoiceStatusKnown: false}
	if err := d.client.do(ctx, http.MethodGet, "/account", nil, &account); err != nil {
		return out, normalizeError("billing", err)
	}
	if err := providers.ValidateMoney(string(account.Account.Balance)); err != nil {
		return out, err
	}
	if err := providers.ValidateMoney(string(account.Account.Pending)); err != nil {
		return out, err
	}
	out.Balance = string(account.Account.Balance)
	out.PendingCharges = string(account.Account.Pending)
	out.GeneratedAt = time.Now().UTC()
	var page struct {
		Invoices []struct {
			ID     json.Number `json:"id"`
			Amount json.Number `json:"amount"`
			Date   string      `json:"date"`
		} `json:"billing_invoices"`
	}
	if err := d.client.do(ctx, http.MethodGet, "/billing/invoices?per_page=20", nil, &page); err != nil {
		return out, normalizeError("billing_invoices", err)
	}
	for _, invoice := range page.Invoices {
		if err := providers.ValidateMoney(string(invoice.Amount)); err != nil {
			return out, err
		}
		out.Invoices = append(out.Invoices, providers.Invoice{ID: string(invoice.ID), Amount: string(invoice.Amount), Date: invoice.Date, Status: "UNKNOWN"})
	}
	return out, nil
}
