package vultr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBillingPreservesAmountsAndUnknownPaymentState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("billing mutation")
		}
		switch r.URL.Path {
		case "/account":
			w.Write([]byte(`{"account":{"balance":-3.001,"pending_charges":0.75}}`))
		case "/billing/invoices":
			w.Write([]byte(`{"billing_invoices":[{"id":123,"amount":3.001,"date":"2026-10-01T00:00:00Z"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := NewClient(server.Client(), "fixture")
	c.base = server.URL
	d := &Driver{client: c}
	b, err := d.Billing(context.Background())
	if err != nil || b.Balance != "-3.001" || len(b.Invoices) != 1 || b.Invoices[0].Status != "UNKNOWN" || b.InvoiceStatusKnown {
		t.Fatal(b, err)
	}
}
func TestSSHInventoryRejectsMissingList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) }))
	defer server.Close()
	c := NewClient(server.Client(), "fixture")
	c.base = server.URL
	if _, err := (&Driver{client: c}).ListSSHKeys(context.Background()); err == nil {
		t.Fatal("missing list accepted")
	}
}
