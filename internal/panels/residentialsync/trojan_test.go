package residentialsync

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/trojantcp"
	"testing"
)

func TestManagedTrojanKeepsExplicitRoutingClasses(t *testing.T) {
	raws := []json.RawMessage{
		json.RawMessage(`{"id":1,"tag":"inbound-443","protocol":"vless","settings":{"clients":[{"id":"v","email":"v@test"}]}}`),
		json.RawMessage(`{"id":2,"tag":"inbound-8443","remark":"dob:trojan-tcp:08443","protocol":"trojan","settings":{"clients":[{"id":"td","email":"td@test","password":"private"},{"id":"tr","email":"tr@test","password":"private2"}]}}`),
		json.RawMessage(`{"id":3,"tag":"manual-trojan","protocol":"trojan","settings":{"clients":[{"email":"manual@test","password":"unmanaged"}]}}`),
	}
	previous := map[string]clientRoute{"v": {ID: "v", Email: "v@test", Class: "DIRECT"}, "td": {ID: "td", Email: "td@test", Class: "DIRECT"}, "tr": {ID: "tr", Email: "tr@test", Class: "RESIDENTIAL"}}
	p := routePolicy{Explicit: true, StrictAllowlist: true, Direct: true, Residential: true, Harden: true, AdsOnly: true, Configured: 1, Proxies: []rp{{Type: "socks5", Tag: "residential-ads-test"}}}
	clients, tags, e := planClients(raws, previous, p)
	if e != nil {
		t.Fatal(e)
	}
	if len(clients) != 3 || len(tags) != 2 {
		t.Fatal("mixed inventory lost clients or admitted unmanaged inlet")
	}
	for _, c := range clients {
		if c.Class != previous[c.ID].Class || c.Effective != c.Class {
			t.Fatal("immutable routing class changed")
		}
	}
	p.Proxies = nil
	clients, _, e = planClients(raws, previous, p)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range clients {
		if c.ID == "tr" && c.Effective != "BLOCKED" {
			t.Fatal("Residential leaked Direct")
		}
	}
}

func TestGeneratedTrojanPassesStrictResidentialSniffing(t *testing.T) {
	p := trojantcp.Policy{Port: 8443, Direct: true, Residential: true}
	credentials, e := trojantcp.NewCredentials("fixture")
	if e != nil {
		t.Fatal(e)
	}
	payload, e := trojantcp.Build(p, "192.0.2.1", credentials.Clients(p))
	if e != nil {
		t.Fatal(e)
	}
	payload["id"] = 2
	payload["tag"] = "inbound-8443"
	raw, _ := json.Marshal(payload)
	if e = validateAdSniffing([]json.RawMessage{raw}, true); e != nil {
		t.Fatal("generated Trojan disables strict Residential routing", e)
	}
	payload["sniffing"].(map[string]any)["routeOnly"] = true
	raw, _ = json.Marshal(payload)
	if validateAdSniffing([]json.RawMessage{raw}, true) == nil {
		t.Fatal("unsafe destination retention accepted")
	}
}
