package jobs

import "testing"

func TestOperationRejectsCrossTenantUse(t *testing.T) {
	o := Operation{AccountID: "a", IdempotencyKey: "create:a:1"}
	if err := o.Authorize("b"); err == nil {
		t.Fatal("cross-tenant operation must fail")
	}
	if err := o.Authorize("a"); err != nil {
		t.Fatal(err)
	}
}
