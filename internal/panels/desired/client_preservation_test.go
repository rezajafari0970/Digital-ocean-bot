package desired

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
	"testing"
)

func TestStructuralUpdatePreservesAllClientsAndUnknownFields(t *testing.T) {
	live := `{"clients":[{"id":"manual","unknown":9007199254740993,"enable":false},{"id":"owned","limitHwid":3}],"unknownSetting":"retain","decryption":"old"}`
	p := realityconfig.Payload{Settings: map[string]any{"clients": []any{map[string]any{"id": "bootstrap"}}, "decryption": "none"}}
	got, err := preserveInboundClients(p, live)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(got.Settings)
	if err != nil {
		t.Fatal(err)
	}
	var observed, want map[string]json.RawMessage
	json.Unmarshal(b, &observed)
	json.Unmarshal([]byte(live), &want)
	if string(observed["clients"]) != string(want["clients"]) || string(observed["unknownSetting"]) != string(want["unknownSetting"]) || string(observed["decryption"]) != `"none"` {
		t.Fatalf("changed client fields: %s", b)
	}
	for _, bad := range []string{`{}`, `{"clients":null}`, `{"clients":{}}`, `bad`} {
		if _, err = preserveInboundClients(p, bad); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}

type preservationExec struct{ post []byte }

func (e *preservationExec) Do(_ context.Context, r sanaei.SessionRequest) (sanaei.SessionResponse, error) {
	if r.Method == "GET" {
		return sanaei.SessionResponse{StatusCode: 200, Body: []byte(`{"success":true,"obj":{"id":1,"settings":{"clients":[{"id":"manual","unknown":9007199254740993}]}}}`)}, nil
	}
	e.post = r.Body
	return sanaei.SessionResponse{StatusCode: 200, Body: []byte(`{"success":true}`)}, nil
}
func TestStructuralUpdateUsesFreshRawClients(t *testing.T) {
	e := &preservationExec{}
	d := panelDeps{exec: e}
	p := realityconfig.Payload{Settings: map[string]any{"clients": []any{map[string]any{"id": "bootstrap"}}}}
	if err := d.Update(context.Background(), 1, p); err != nil {
		t.Fatal(err)
	}
	var obj struct {
		Settings struct {
			Clients []struct {
				ID      string
				Unknown json.Number
			}
		}
	}
	if err := json.Unmarshal(e.post, &obj); err != nil {
		t.Fatal(err)
	}
	if len(obj.Settings.Clients) != 1 || obj.Settings.Clients[0].ID != "manual" || obj.Settings.Clients[0].Unknown != "9007199254740993" {
		t.Fatalf("client overwritten: %s", e.post)
	}
}
