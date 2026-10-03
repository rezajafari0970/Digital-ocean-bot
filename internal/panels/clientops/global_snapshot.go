package clientops

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

// One fresh global snapshot replaces O(batch) remote round trips while keeping
// UUID/email conflicts and global inbound sharing fail-closed.
func globalWanted(ctx context.Context, rt *sanaei.PanelRuntime, inbound int64, wanted []sanaei.Client) (map[string]sanaei.GlobalClient, error) {
	records, err := sanaei.ReadGlobalClientsSession(ctx, rt.Session.Exec)
	if err != nil {
		return nil, err
	}
	return selectGlobalWanted(records, inbound, wanted)
}
func selectGlobalWanted(records []sanaei.GlobalClient, inbound int64, wanted []sanaei.Client) (map[string]sanaei.GlobalClient, error) {
	ids, emails := map[string]string{}, map[string]string{}
	for _, c := range wanted {
		ids[c.ID] = c.Email
		emails[c.Email] = c.ID
	}
	found := map[string]sanaei.GlobalClient{}
	for _, g := range records {
		email, byID := ids[g.UUID]
		id, byEmail := emails[g.Email]
		if !byID && !byEmail {
			continue
		}
		if !byID || !byEmail || email != g.Email || id != g.UUID {
			return nil, ErrClientConflict
		}
		if _, duplicate := found[g.UUID]; duplicate {
			return nil, ErrClientConflict
		}
		for _, location := range g.InboundIDs {
			if location != inbound {
				return nil, ErrClientConflict
			}
		}
		found[g.UUID] = g
	}
	return found, nil
}
