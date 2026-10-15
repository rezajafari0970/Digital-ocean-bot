package adminapi

import (
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"testing"
)

func TestOutputRefreshCoversFleetWithinFreshnessBudget(t *testing.T) {
	panels := make([]readyworker.Panel, 36)
	for i := range panels {
		panels[i].ID = fmt.Sprintf("%03d", i)
	}
	after := ""
	seen := map[string]bool{}
	// Completed batches free eight slots each second; each panel must be
	// admitted within five ticks, well within the unchanged 15-second TTL.
	for tick := 0; tick < 5; tick++ {
		slots := 8
		after = scheduleOutputPanels(panels, after, func(p readyworker.Panel) bool {
			if slots == 0 {
				return false
			}
			slots--
			seen[p.ID] = true
			return true
		})
	}
	if len(seen) != 36 {
		t.Fatalf("only %d/36 panels refreshed within five ticks", len(seen))
	}
}
func TestOutputRefreshCursorTracksAdmissionsAndChangingLists(t *testing.T) {
	panels := []readyworker.Panel{{ID: "a"}, {ID: "c"}, {ID: "e"}}
	after := scheduleOutputPanels(panels, "c", func(readyworker.Panel) bool { return false })
	if after != "c" {
		t.Fatalf("rejection moved cursor: %q", after)
	}
	after = scheduleOutputPanels(panels, "b", func(p readyworker.Panel) bool { return p.ID == "e" })
	if after != "e" {
		t.Fatal(after)
	}
	var order []string
	panels = []readyworker.Panel{{ID: "a"}, {ID: "d"}}
	after = scheduleOutputPanels(panels, after, func(p readyworker.Panel) bool { order = append(order, p.ID); return true })
	if after != "d" || fmt.Sprint(order) != "[a d]" {
		t.Fatal(after, order)
	}
	if scheduleOutputPanels(nil, after, func(readyworker.Panel) bool { panic("empty") }) != after {
		t.Fatal("empty changed cursor")
	}
}

func TestOutputRefreshSlowAndBusyPanelsDoNotStarveOthers(t *testing.T) {
	panels := make([]readyworker.Panel, 36)
	for i := range panels {
		panels[i].ID = fmt.Sprintf("%03d", 35-i)
	}
	running := map[string]int{}
	seen := map[string]bool{}
	after := ""
	for tick := 0; tick < 10; tick++ {
		for id, done := range running {
			if done <= tick {
				delete(running, id)
			}
		}
		after = scheduleOutputPanels(panels, after, func(p readyworker.Panel) bool {
			if _, busy := running[p.ID]; busy || len(running) == 8 {
				return false
			}
			duration := 2
			if p.ID == "000" {
				duration = 5
			}
			running[p.ID] = tick + duration
			seen[p.ID] = true
			return true
		})
	}
	if len(seen) != 36 {
		t.Fatalf("slow panel starved fleet: %d/36", len(seen))
	}
	if panels[0].ID != "035" {
		t.Fatal("mutated source ordering")
	}
}
