package cleanup

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"sort"
)

type Observed struct {
	Clients  map[string]string
	Inbounds map[int64]string
	Attached int
}

func Observe(ctx context.Context, exec sanaei.SessionExecutor) (Observed, error) {
	out := Observed{Clients: map[string]string{}, Inbounds: map[int64]string{}}
	clients, err := sanaei.ReadGlobalClientsSession(ctx, exec)
	if err != nil {
		return out, err
	}
	for _, c := range clients {
		if c.Email == "" {
			return out, errors.New("missing global client identity")
		}
		if _, ok := out.Clients[c.Email]; ok {
			return out, errors.New("duplicate global email")
		}
		out.Clients[c.Email] = c.UUID
	}
	raws, err := sanaei.ReadRawInboundList(ctx, exec)
	if err != nil {
		return out, err
	}
	for _, raw := range raws {
		var in struct {
			ID            int64
			Port          int
			Tag, Protocol string
			Settings      json.RawMessage
		}
		if json.Unmarshal(raw, &in) != nil || in.ID <= 0 || in.Port <= 0 || in.Protocol == "" {
			return out, errors.New("invalid inbound identity")
		}
		if _, ok := out.Inbounds[in.ID]; ok {
			return out, errors.New("duplicate inbound")
		}
		identity, _ := json.Marshal([]any{in.Port, in.Tag, in.Protocol})
		out.Inbounds[in.ID] = fmt.Sprintf("%x", sha256.Sum256(identity))
		var str string
		b := in.Settings
		if json.Unmarshal(b, &str) == nil {
			b = []byte(str)
		}
		var settings struct{ Clients []struct{ ID, Email string } }
		if json.Unmarshal(b, &settings) != nil {
			return out, errors.New("invalid inbound settings")
		}
		for _, c := range settings.Clients {
			out.Attached++
			id, exists := out.Clients[c.Email]
			if !exists || (c.ID != "" && id != c.ID) {
				return out, errors.New("global and inbound client inventories disagree")
			}
		}
	}
	return out, nil
}

type clientPlan struct {
	Email, ID string
	Absent    bool
}
type inboundPlan struct {
	ID       int64
	Identity string
	Absent   bool
}

func validateObserved(o Observed, clients []clientPlan, inbounds []inboundPlan) error {
	cm := map[string]clientPlan{}
	for _, c := range clients {
		cm[c.Email] = c
	}
	im := map[int64]inboundPlan{}
	for _, i := range inbounds {
		im[i.ID] = i
	}
	for email, id := range o.Clients {
		c, ok := cm[email]
		if !ok || id != c.ID || c.Absent {
			return errors.New("client identity changed after cleanup plan")
		}
	}
	for id, identity := range o.Inbounds {
		i, ok := im[id]
		if !ok || identity != i.Identity || i.Absent {
			return errors.New("inbound identity changed after cleanup plan")
		}
	}
	return nil
}
func mutationChunk(ctx context.Context, exec sanaei.SessionExecutor, before Observed) (Observed, error) {
	if len(before.Clients) > 0 {
		emails := make([]string, 0, len(before.Clients))
		for e := range before.Clients {
			emails = append(emails, e)
		}
		sort.Strings(emails)
		if len(emails) > 100 {
			emails = emails[:100]
		}
		_, _ = sanaei.BulkDeleteClientsSession(ctx, exec, emails)
		after, err := Observe(ctx, exec)
		if err != nil {
			return after, fmt.Errorf("client delete outcome unknown: %w", err)
		}
		for _, e := range emails {
			if _, present := after.Clients[e]; present {
				return after, errors.New("some planned clients remain; cleanup paused")
			}
		}
		return after, nil
	}
	if before.Attached != 0 {
		return before, errors.New("attached clients remain")
	}
	ids := make([]int64, 0, len(before.Inbounds))
	for id := range before.Inbounds {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) == 0 {
		return before, nil
	}
	_, _ = exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: fmt.Sprintf("panel/api/inbounds/del/%d", ids[0]), TimeoutSeconds: 20})
	after, err := Observe(ctx, exec)
	if err != nil {
		return after, fmt.Errorf("inbound delete outcome unknown: %w", err)
	}
	if _, present := after.Inbounds[ids[0]]; present {
		return after, errors.New("planned inbound remains; cleanup paused")
	}
	return after, nil
}
