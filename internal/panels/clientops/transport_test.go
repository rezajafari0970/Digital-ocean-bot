package clientops

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"testing"
)

func TestXHTTPPlansEmptyFlowAndRejectsVisionBeforePOST(t *testing.T) {
	db, j, rt, state := lifecycleFixture(t)
	_ = db
	state.network = "xhttp"
	state.clients = map[string]sanaei.Client{} // Isolated fresh listener.
	if !planLife(t, j, rt) {
		t.Fatal("not planned")
	}
	job, ok, e := j.Claim(context.Background())
	if e != nil || !ok {
		t.Fatal(e)
	}
	var p BulkPayload
	if json.Unmarshal(job.Payload, &p) != nil {
		t.Fatal("payload")
	}
	for _, c := range p.Clients {
		if c.Flow != "" {
			t.Fatal("Vision planned")
		}
	}
	invalid := job
	p.Clients[0].Flow = "xtls-rprx-vision"
	invalid.Payload, _ = json.Marshal(p)
	if e := (Executor{Journal: j}).executeRuntime(context.Background(), rt, invalid); !errors.Is(e, ErrClientConflict) {
		t.Fatal("Vision accepted", e)
	}
	if state.postCreates != 0 {
		t.Fatal("incompatible POST")
	}
	if e := (Executor{Journal: j}).executeRuntime(context.Background(), rt, job); e != nil {
		t.Fatal(e)
	}
	if state.postCreates != 1 {
		t.Fatal(state.postCreates)
	}
}
