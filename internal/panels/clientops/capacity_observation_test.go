package clientops

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configprofiles"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"testing"
	"time"
)

func TestCapacityObservationDoesNotMaskManagedClassDeficits(t *testing.T) {
	profiles := []configprofiles.Profile{{Class: "DIRECT", Enabled: true, Ports: []int{443}, Target: 1}, {Class: "RESIDENTIAL", Enabled: true, Ports: []int{443}, Target: 3}}
	clients := map[string]LifecycleClient{}
	managed := map[string]bool{}
	classes := map[string]string{}
	for _, id := range []string{"d1", "d2", "d3", "manual"} {
		clients[id] = LifecycleClient{Client: sanaei.Client{ID: id, Email: id, Enable: true}}
		if id != "manual" {
			managed[id] = true
			classes[id] = "DIRECT"
		}
	}
	counts, err := ObserveCapacity(profiles, 443, clients, managed, classes, time.Now())
	if err != nil || counts.Target != 5 || counts.Active != 4 || counts.Deficit != 3 {
		t.Fatal(counts, err)
	}
	c := clients["manual"]
	c.Client.TotalGB = 100
	clients["manual"] = c
	if _, err = ObserveCapacity(profiles, 443, clients, managed, classes, time.Now()); err == nil {
		t.Fatal("missing traffic accepted")
	}
	c.Traffic = &sanaei.ClientTraffic{Email: "manual", Up: 100}
	clients["manual"] = c
	counts, err = ObserveCapacity(profiles, 443, clients, managed, classes, time.Now())
	if err != nil || counts.Quota != 1 || counts.Active != 3 || counts.Target != 4 || counts.Deficit != 3 {
		t.Fatal(counts, err)
	}
}
