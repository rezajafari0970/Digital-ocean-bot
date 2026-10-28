package kamatera

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestKamateraRegionalUncertaintyRejectsOnlyNewDispatch(t *testing.T) {
	j := &memoryJournal{regionalError: providers.ErrRegionalCreateUnresolved}
	posts := 0
	d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.Query().Get("sizes") == "1":
			return reply(sizeJSON), nil
		case r.URL.Query().Get("images") == "1":
			return reply(imageJSON), nil
		case r.URL.Path == "/service/servers" || r.URL.Path == "/service/server/info":
			return reply("[]"), nil
		case r.Method == "POST" && r.URL.Path == "/service/server":
			posts++
			return reply("[12345]"), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	}, j)
	r := providers.CreateServerRequest{Identity: identity, RegionID: "EU", PlanID: "1B-1024-20", ImageID: "EU:ubuntu-24.04-64bit", SSHAuthorizedKeys: []string{fixtureKey(t)}}
	out, err := d.CreateServer(context.Background(), r)
	if !errors.Is(err, providers.ErrRegionalCreateUnresolved) || !providers.IsClass(err, providers.ErrorRegionCapacity) || !out.ComputeNotDispatched || out.Outcome != providers.OutcomeRejected || posts != 0 || j.record.Attempted || j.record.Intent != "" {
		t.Fatalf("regional guard dispatched or froze an unattempted intent: %+v %v %+v posts=%d", out, err, j.record, posts)
	}
	// After the other operation is acknowledged, the untouched intent may proceed.
	j.regionalError = nil
	out, err = d.CreateServer(context.Background(), r)
	if !providers.IsClass(err, providers.ErrorAmbiguousOutcome) || out.OperationID != "12345" || posts != 1 {
		t.Fatalf("admission did not reopen: %+v %v posts=%d", out, err, posts)
	}
	// Another unresolved peer cannot stop observation or replay protection.
	j.regionalError = providers.ErrRegionalCreateUnresolved
	out, err = d.CreateServer(context.Background(), r)
	if !providers.IsClass(err, providers.ErrorAmbiguousOutcome) || errors.Is(err, providers.ErrRegionalCreateUnresolved) || posts != 1 {
		t.Fatalf("existing intent changed: %+v %v posts=%d", out, err, posts)
	}
}
