package ipam

import (
	"fmt"
)

// AcquisitionContract identifies exact provider acquisition receipts. DO IPv4\n// creation is 202: a separate correlated resource GET/readback is mandatory.
// No generic 4xx definitive-noncreation contract is enabled: intermediary replies
// and unclassified errors retain the obligation until explicit reconciliation.
// These are receipt contracts, not executable drivers or dispatch authority.
func AcquisitionContract(provider, product string, family int) string {
	supported := false
	switch provider {
	case "digitalocean", "vultr":
		supported = product == "reserved" && (family == 4 || family == 6)
	case "linode":
		supported = (product == "public" && family == 4) || (product == "routed" && family == 6)
	case "upcloud":
		supported = (product == "public" && (family == 4 || family == 6)) || (product == "floating" && family == 4)
	}
	if !supported {
		return ""
	}
	if provider == "digitalocean" && family == 4 {
		return "digitalocean/reserved/ipv4/ALLOCATE/readback-200/v1"
	}
	status := 201
	if provider == "linode" {
		status = 200
	}
	return fmt.Sprintf("%s/%s/ipv%d/ALLOCATE/created-%d/v1", provider, product, family, status)
}
func validateOutcome(provider, product, action string, family int, r DispatchReceipt) error {
	switch r.Outcome {
	case "UNKNOWN":
		return nil
	case "ACCEPTED", "ACKNOWLEDGED":
		if r.HTTPStatus >= 200 && r.HTTPStatus < 300 {
			return nil
		}
	case "ACQUIRED":
		expected := AcquisitionContract(provider, product, family)
		status := 201
		if provider == "linode" || (provider == "digitalocean" && family == 4) {
			status = 200
		}
		if action == "ALLOCATE" && expected != "" && r.Contract == expected && r.HTTPStatus == status && r.ResourceID != "" && (!(provider == "digitalocean" && family == 4) || r.AcknowledgementKey != "") {
			return nil
		}
		// No provider rejection contract has yet been audited sufficiently for discharge.
	}
	return ErrEvidence
}
