package clientops

import (
	"context"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
)

// InboundFlow reads the same authenticated snapshot used by lifecycle planning.
// Default policy revisions do not reinterpret queued payloads. Explicit transport
// applications drain queued work and fence admission before changing a pin.
func InboundFlow(ctx context.Context, rt *sanaei.PanelRuntime, id int64) (string, error) {
	flow, _, err := inboundContract(ctx, rt, id)
	return flow, err
}
func inboundContract(ctx context.Context, rt *sanaei.PanelRuntime, id int64) (string, string, error) {
	if rt == nil || rt.Session == nil {
		return "", "", ErrInvalidRequest
	}
	raws, e := rt.Session.Snapshot(ctx)
	if e != nil {
		return "", "", e
	}
	found := false
	flow := ""
	network := ""
	for _, raw := range raws {
		var in struct {
			Settings json.RawMessage
			ID       int64
			Enable   bool
			Protocol string
			Stream   json.RawMessage `json:"streamSettings"`
		}
		if json.Unmarshal(raw, &in) != nil {
			return "", "", ErrVerify
		}
		if in.ID != id {
			continue
		}
		if found || !in.Enable || in.Protocol != "vless" {
			return "", "", ErrInboundMissing
		}
		found = true
		var st struct{ Network, Security string }
		if json.Unmarshal(in.Stream, &st) != nil || st.Security != "reality" || !transportprofile.Supported(st.Network) {
			return "", "", ErrClientConflict
		}
		network = st.Network
		var settings map[string]any
		if json.Unmarshal(in.Settings, &settings) != nil {
			return "", "", ErrVerify
		}
		flow, e = transportprofile.InboundServerFlow(network, settings)
		if e != nil {
			return "", "", ErrClientConflict
		}
	}
	if !found {
		return "", "", ErrInboundMissing
	}
	return flow, network, nil
}
func validateJobTransport(ctx context.Context, rt *sanaei.PanelRuntime, job Job) error {
	if job.Kind != KindCreate && job.Kind != KindBulkCreate && job.Kind != KindUpdate {
		return nil
	}
	if err := rt.Session.InvalidateContext(ctx); err != nil {
		return err
	}
	flow, network, e := inboundContract(ctx, rt, job.InboundID)
	if e != nil {
		return e
	}
	if network == "tcp" {
		return nil
	} // Existing TCP semantics are unchanged.
	switch job.Kind {
	case KindCreate:
		c, e := payloadClient(job.Payload)
		if e != nil {
			return e
		}
		if c.Flow != flow {
			return ErrClientConflict
		}
	case KindBulkCreate:
		var p BulkPayload
		if json.Unmarshal(job.Payload, &p) != nil {
			return ErrInvalidRequest
		}
		for _, c := range p.Clients {
			if c.Flow != flow {
				return ErrClientConflict
			}
		}
	case KindUpdate:
		p, e := payloadPatch(job.Payload)
		if e != nil {
			return e
		}
		if p.Flow != nil && *p.Flow != flow {
			return ErrClientConflict
		}
	}
	return nil
}
