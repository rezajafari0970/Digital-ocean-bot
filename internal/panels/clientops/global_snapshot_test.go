package clientops

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"testing"
)

func TestGlobalSnapshotRejectsConflictsSharingAndDuplicates(t *testing.T) {
	c := sanaei.Client{ID: "a", Email: "owned"}
	g := sanaei.GlobalClient{UUID: "a", Email: "owned", InboundIDs: []int64{1}}
	if found, e := selectGlobalWanted([]sanaei.GlobalClient{g}, 1, []sanaei.Client{c}); e != nil || len(found) != 1 {
		t.Fatal(found, e)
	}
	for _, records := range [][]sanaei.GlobalClient{{g, g}, {{UUID: "b", Email: "owned"}}, {{UUID: "a", Email: "other"}}, {{UUID: "a", Email: "owned", InboundIDs: []int64{1, 2}}}} {
		if _, e := selectGlobalWanted(records, 1, []sanaei.Client{c}); e == nil {
			t.Fatal("unsafe global snapshot")
		}
	}
}
