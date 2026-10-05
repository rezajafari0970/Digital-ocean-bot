package resolverpolicy

import (
	"encoding/json"
	"net/netip"
	"testing"
)

func TestActiveResolversAreUniqueVerifiedPublicIPv4AndUnfiltered(t *testing.T) {
	var catalog struct {
		Active    []string
		Resolvers []struct {
			Address, Profile, Status string
			Active                   bool
		}
	}
	if err := json.Unmarshal(Catalog, &catalog); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	enabled := map[string]bool{}
	for _, r := range catalog.Resolvers {
		ip, err := netip.ParseAddr(r.Address)
		if err != nil || seen[ip.String()] {
			t.Fatal("invalid or repeated resolver", r.Address)
		}
		seen[ip.String()] = true
		if r.Active {
			if !ip.Is4() || !ip.IsGlobalUnicast() || ip.IsPrivate() || r.Status != "answered" || r.Profile != "unfiltered" {
				t.Fatal("unsafe active resolver", r.Address)
			}
			enabled[r.Address] = true
		}
	}
	if len(catalog.Resolvers) != 2 || len(Active()) != 2 || len(Active()) != len(enabled) {
		t.Fatal("unbounded or inconsistent active pool")
	}
	allowed := map[string]bool{"1.1.1.1": true, "8.8.8.8": true}
	for _, a := range Active() {
		if !allowed[a] {
			t.Fatal("unrequested resolver", a)
		}
		if !enabled[a] {
			t.Fatal("pool missing evidence", a)
		}
	}
}
