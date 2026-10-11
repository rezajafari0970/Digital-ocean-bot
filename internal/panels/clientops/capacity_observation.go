package clientops

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configprofiles"
	"time"
)

// CapacityObservation describes one native observation. Unmanaged clients are
// preserved in the total but cannot satisfy either managed class's target.
type CapacityObservation struct{ Target, Active, Deficit, Expired, Quota int }

func ObserveCapacity(profiles []configprofiles.Profile, port int, clients map[string]LifecycleClient, managed map[string]bool, classes map[string]string, now time.Time) (CapacityObservation, error) {
	out := CapacityObservation{Target: configprofiles.Target(profiles, port)}
	active := map[string]int{}
	for id, c := range clients {
		reason, err := c.InactiveReason(now)
		if err != nil {
			return CapacityObservation{}, err
		}
		switch reason {
		case "EXPIRED":
			out.Expired++
		case "QUOTA":
			out.Quota++
		}
		if reason != "" {
			continue
		}
		out.Active++
		if !managed[id] {
			out.Target++
			continue
		}
		class := classes[id]
		if class == "" {
			class = "RESIDENTIAL"
		}
		active[class]++
	}
	for _, p := range profiles {
		if p.Enabled && p.Applies(port) {
			out.Deficit += max(p.Target-active[p.Class], 0)
		}
	}
	return out, nil
}
