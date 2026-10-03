package usercapacity

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

func TestDesiredOwnedClientUsesCreationEpoch(t *testing.T) {
	created := time.Unix(1700000000, 0).UTC()
	own := ownedPolicy{Email: "u-marker-12345678", Marker: "marker", CreatedAt: created}
	in := sanaei.Client{ID: "c1", Email: own.Email, Enable: true, TotalGB: 1, ExpiryTime: 99, LimitIP: 1}
	got, changed := desiredOwnedClient(in, own, 500, 3600, 3)
	if !changed {
		t.Fatal("expected drift")
	}
	if got.TotalGB != 500 || got.LimitIP != 3 {
		t.Fatalf("got=%+v", got)
	}
	want := created.Add(time.Hour).UnixMilli()
	if got.ExpiryTime != want {
		t.Fatalf("expiry=%d want=%d", got.ExpiryTime, want)
	}
}

func TestDesiredOwnedClientLifetimeZeroIsInfinite(t *testing.T) {
	own := ownedPolicy{Email: "u-marker-12345678", Marker: "marker", CreatedAt: time.Now()}
	in := sanaei.Client{Email: own.Email, ExpiryTime: 123}
	got, changed := desiredOwnedClient(in, own, 0, 0, 0)
	if !changed || got.ExpiryTime != 0 {
		t.Fatalf("got=%+v changed=%v", got, changed)
	}
}

func TestDesiredOwnedClientRejectsMarkerMismatch(t *testing.T) {
	in := sanaei.Client{Email: "ordinary", TotalGB: 7, ExpiryTime: 8, LimitIP: 9}
	got, changed := desiredOwnedClient(in, ownedPolicy{Email: "ordinary", Marker: "marker"}, 1, 1, 1)
	if changed || got != in {
		t.Fatal("manual/unmarked client must remain untouched")
	}
}

func TestVerifyOwnedPolicySnapshot(t *testing.T) {
	settings, _ := json.Marshal(map[string]any{"clients": []sanaei.Client{{ID: "c1", Email: "u-marker-12345678", TotalGB: 5, ExpiryTime: 7, LimitIP: 2}}})
	raw, _ := json.Marshal(map[string]any{"id": 1, "settings": json.RawMessage(settings)})
	want := map[string]sanaei.Client{"c1": {ID: "c1", Email: "u-marker-12345678", TotalGB: 5, ExpiryTime: 7, LimitIP: 2}}
	if err := verifyOwnedPolicySnapshot([]json.RawMessage{raw}, 1, want, nil); err != nil {
		t.Fatal(err)
	}
	if err := verifyOwnedPolicySnapshot([]json.RawMessage{raw}, 1, want, []string{"c1"}); err == nil {
		t.Fatal("expected deletion verification failure")
	}
}
