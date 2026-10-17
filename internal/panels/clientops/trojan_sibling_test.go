package clientops

import (
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"testing"
)

func TestTrojanSiblingDoesNotBreakVLESSCapacityObservation(t *testing.T) {
	wanted := []sanaei.Client{{ID: "vless-id", Email: "vless@test", Enable: true}}
	v := json.RawMessage(`{"id":1,"enable":true,"protocol":"vless","settings":{"clients":[{"id":"vless-id","email":"vless@test","enable":true}]}}`)
	tr := json.RawMessage(`{"id":2,"enable":true,"protocol":"trojan","settings":{"clients":[{"id":"trojan-id","email":"trojan@test","password":"not-an-id","enable":true}]}}`)
	observed, missing, e := bulkObserved([]json.RawMessage{v, tr}, 1, wanted)
	if e != nil || len(observed) != 1 || len(missing) != 0 {
		t.Fatal("Trojan interfered with VLESS observation", e)
	}
	conflict := json.RawMessage(`{"id":2,"enable":true,"protocol":"trojan","settings":{"clients":[{"id":"trojan-id","email":"vless@test","password":"not-an-id","enable":true}]}}`)
	if _, _, e = bulkObserved([]json.RawMessage{v, conflict}, 1, wanted); e == nil {
		t.Fatal("cross-protocol email collision accepted")
	}
}
