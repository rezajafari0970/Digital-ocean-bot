package clientops

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
)

// InboundFlow reads the same authenticated snapshot used by lifecycle planning.
// Transport is immutable for pinned listeners; new policy revisions never
// reinterpret queued payloads or convert an existing listener.
func InboundFlow(ctx context.Context, rt *sanaei.PanelRuntime, id int64) (string, error) {
	if rt == nil || rt.Session == nil {
		return "", ErrInvalidRequest
	}
	raws, e := rt.Session.Snapshot(ctx)
	if e != nil {
		return "", e
	}
	found := false
	flow := ""
	for _, raw := range raws {
		var in struct {
			ID       int64
			Enable   bool
			Protocol string
			Stream   json.RawMessage `json:"streamSettings"`
		}
		if json.Unmarshal(raw, &in) != nil {
			return "", ErrVerify
		}
		if in.ID != id {
			continue
		}
		if found || !in.Enable || in.Protocol != "vless" {
			return "", ErrInboundMissing
		}
		found = true
		var st struct{ Network, Security string }
		if json.Unmarshal(in.Stream, &st) != nil || st.Security != "reality" || !transportprofile.Supported(st.Network) {
			return "", ErrClientConflict
		}
		flow = transportprofile.Flow(st.Network)
	}
	if !found {
		return "", ErrInboundMissing
	}
	return flow, nil
}
func validateJobTransport(ctx context.Context, rt *sanaei.PanelRuntime, job Job) error {
	if job.Kind != KindCreate && job.Kind != KindBulkCreate && job.Kind != KindUpdate {
		return nil
	}
	rt.Session.Invalidate()
	flow, e := InboundFlow(ctx, rt, job.InboundID)
	if e != nil {
		return e
	}
	if flow != "" {
		return nil
	} // Existing TCP semantics are unchanged.
	switch job.Kind {
	case KindCreate:
		c, e := payloadClient(job.Payload)
		if e != nil {
			return e
		}
		if c.Flow != "" {
			return ErrClientConflict
		}
	case KindBulkCreate:
		var p BulkPayload
		if json.Unmarshal(job.Payload, &p) != nil {
			return ErrInvalidRequest
		}
		for _, c := range p.Clients {
			if c.Flow != "" {
				return ErrClientConflict
			}
		}
	case KindUpdate:
		p, e := payloadPatch(job.Payload)
		if e != nil {
			return e
		}
		if p.Flow != nil && *p.Flow != "" {
			return ErrClientConflict
		}
	}
	return nil
}
