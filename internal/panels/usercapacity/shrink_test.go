package usercapacity

import (
	"fmt"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func TestOwnedShrinkCandidatesNeverSelectManualClient(t *testing.T) {
	active := []sanaei.Client{
		{ID: "manual", Email: "manual", Enable: true},
		{ID: "o1", Email: "u-m-o1", Enable: true},
		{ID: "o2", Email: "u-m-o2", Enable: true},
	}
	owned := map[string]ownedPolicy{
		"o1": {Email: "u-m-o1", Marker: "m", CreatedAt: time.Now()},
		"o2": {Email: "u-m-o2", Marker: "m", CreatedAt: time.Now()},
	}
	got := ownedShrinkCandidates(active, owned, 1, 32)
	if len(got) != 2 {
		t.Fatalf("got=%v", got)
	}
	for _, id := range got {
		if id == "manual" {
			t.Fatal("manual client selected")
		}
	}
}

func TestOwnedShrinkCandidatesBounded(t *testing.T) {
	active := make([]sanaei.Client, 0, 50)
	owned := map[string]ownedPolicy{}
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("o%02d", i)
		email := "u-m-" + id
		active = append(active, sanaei.Client{ID: id, Email: email, Enable: true})
		owned[id] = ownedPolicy{Email: email, Marker: "m"}
	}
	if got := ownedShrinkCandidates(active, owned, 1, 32); len(got) != 32 {
		t.Fatalf("len=%d", len(got))
	}
}

func TestOwnedShrinkCandidatesIgnoresDisabled(t *testing.T) {
	active := []sanaei.Client{{ID: "base", Email: "base", Enable: true}, {ID: "disabled", Email: "u-m-disabled", Enable: false}}
	owned := map[string]ownedPolicy{"disabled": {Email: "u-m-disabled", Marker: "m"}}
	if got := ownedShrinkCandidates(active, owned, 1, 32); len(got) != 0 {
		t.Fatalf("got=%v", got)
	}
}
