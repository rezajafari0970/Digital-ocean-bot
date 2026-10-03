package digitalocean

import (
	"context"
	"net/http"
	"testing"
)

func TestBillingDoesNotInventInvoicePaymentStatus(t *testing.T) {
	c := fixtureClient(t, func(r *http.Request) (int, string) {
		if r.Method != "GET" {
			t.Fatal("billing mutation")
		}
		switch r.URL.Path {
		case "/v2/customers/my/balance":
			return 200, `{"account_balance":"3.00","month_to_date_usage":"0.75","generated_at":"2026-10-03T00:00:00Z"}`
		case "/v2/customers/my/invoices":
			return 200, `{"invoices":[{"invoice_uuid":"a","amount":"3.00","invoice_period":"2026-09"}]}`
		}
		t.Fatal(r.URL.Path)
		return 500, ""
	})
	d, _ := NewDriver(c)
	b, err := d.Billing(context.Background())
	if err != nil || b.Balance != "3.00" || len(b.Invoices) != 1 || b.Invoices[0].Status != "UNKNOWN" || b.InvoiceStatusKnown {
		t.Fatal(b, err)
	}
}
func TestBillingMissingBalanceIsNotZero(t *testing.T) {
	c := fixtureClient(t, func(*http.Request) (int, string) { return 200, `{}` })
	d, _ := NewDriver(c)
	if _, err := d.Billing(context.Background()); err == nil {
		t.Fatal("missing financial data accepted")
	}
}
