package ipam

import "testing"

func TestAcquisitionOutcomeContracts(t *testing.T) {
	for _, c := range []struct {
		provider, product string
		family, status    int
	}{
		{"digitalocean", "reserved", 4, 200}, {"digitalocean", "reserved", 6, 201}, {"vultr", "reserved", 4, 201}, {"vultr", "reserved", 6, 201}, {"linode", "public", 4, 200}, {"linode", "routed", 6, 200}, {"upcloud", "public", 4, 201}, {"upcloud", "public", 6, 201}, {"upcloud", "floating", 4, 201},
	} {
		t.Run(c.provider+c.product+string(rune('0'+c.family)), func(t *testing.T) {
			r := DispatchReceipt{Outcome: "ACQUIRED", HTTPStatus: c.status, ResourceID: "fixture", Contract: AcquisitionContract(c.provider, c.product, c.family)}
			if c.provider == "digitalocean" && c.family == 4 {
				r.AcknowledgementKey = "prior-202"
			}
			if e := validateOutcome(c.provider, c.product, "ALLOCATE", c.family, r); e != nil {
				t.Fatal(e)
			}
			r.HTTPStatus = 202
			if e := validateOutcome(c.provider, c.product, "ALLOCATE", c.family, r); e == nil {
				t.Fatal("ack alone acquired resource")
			}
			r.HTTPStatus = c.status
			if e := validateOutcome(c.provider, c.product, "RELEASE", c.family, r); e == nil {
				t.Fatal("creation verdict reused for release")
			}
		})
	}
	if AcquisitionContract("upcloud", "floating", 6) != "" {
		t.Fatal("unsupported floating IPv6")
	}
}
