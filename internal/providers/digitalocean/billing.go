package digitalocean

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"time"
)

func (d *Driver) Billing(ctx context.Context) (providers.Billing, error) {
	var balance struct {
		Balance   string    `json:"account_balance"`
		Usage     string    `json:"month_to_date_usage"`
		Generated time.Time `json:"generated_at"`
	}
	out := providers.Billing{Currency: "USD", Invoices: []providers.Invoice{}, InvoiceStatusKnown: false}
	if err := d.client.get(ctx, "/customers/my/balance", &balance); err != nil {
		return out, normalizeError("billing", err)
	}
	if err := providers.ValidateMoney(balance.Balance); err != nil {
		return out, err
	}
	if err := providers.ValidateMoney(balance.Usage); err != nil {
		return out, err
	}
	out.Balance = balance.Balance
	out.PendingCharges = balance.Usage
	out.GeneratedAt = balance.Generated
	var page struct {
		Invoices []struct {
			ID     string `json:"invoice_uuid"`
			Amount string `json:"amount"`
			Period string `json:"invoice_period"`
		} `json:"invoices"`
	}
	if err := d.client.get(ctx, "/customers/my/invoices?per_page=20", &page); err != nil {
		return out, normalizeError("billing_invoices", err)
	}
	for _, invoice := range page.Invoices {
		if err := providers.ValidateMoney(invoice.Amount); err != nil {
			return out, err
		}
		out.Invoices = append(out.Invoices, providers.Invoice{ID: invoice.ID, Amount: invoice.Amount, Date: invoice.Period, Status: "UNKNOWN"})
	}
	return out, nil
}
