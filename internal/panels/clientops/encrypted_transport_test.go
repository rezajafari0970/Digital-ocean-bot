package clientops

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
)

func TestEncryptedXHTTPPlansVisionAndRejectsStaleEmptyFlow(t *testing.T) {
	_, j, rt, state := lifecycleFixture(t)
	state.network = "xhttp"
	state.clients = map[string]sanaei.Client{}
	state.flow = transportprofile.Vision
	private := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))
	var err error
	state.decryption, _, err = transportprofile.EncryptionPair(private, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !planLife(t, j, rt) {
		t.Fatal("encrypted plan missing")
	}
	job, ok, err := j.Claim(context.Background())
	if err != nil || !ok {
		t.Fatal(err)
	}
	var p BulkPayload
	if json.Unmarshal(job.Payload, &p) != nil || len(p.Clients) == 0 {
		t.Fatal("payload")
	}
	for _, c := range p.Clients {
		if c.Flow != transportprofile.Vision {
			t.Fatal("wrong encrypted flow")
		}
	}
	invalid := job
	p.Clients[0].Flow = ""
	invalid.Payload, _ = json.Marshal(p)
	if err = (Executor{Journal: j}).executeRuntime(context.Background(), rt, invalid); !errors.Is(err, ErrClientConflict) {
		t.Fatal("stale empty flow accepted", err)
	}
	if state.postCreates != 0 {
		t.Fatal("incompatible POST")
	}
	if err = (Executor{Journal: j}).executeRuntime(context.Background(), rt, job); err != nil {
		t.Fatal(err)
	}
	if state.postCreates != 1 {
		t.Fatal("compatible job not applied")
	}
}
