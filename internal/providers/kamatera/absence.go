package kamatera

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

// confirmInventoryAbsence uses successful, authenticated complete reads.
// A failed exact lookup contributes no absence evidence. Both inventories must
// agree on every identity, then the existing durable age/repetition fence applies.
func (d *Driver) confirmInventoryAbsence(ctx context.Context, id string) error {
	basic, err := d.basicServers(ctx)
	if err != nil {
		return err
	}
	identities := make(map[string]bool, len(basic))
	for _, x := range basic {
		if x.ID == id {
			return d.absenceDisagreement(ctx, id)
		}
		identities[x.ID] = true
	}
	detailed, err := d.rawServers(ctx, map[string]string{"name": ".*"})
	if err != nil {
		return err
	}
	for _, x := range detailed {
		if x.ID == id {
			return d.absenceDisagreement(ctx, id)
		}
		if !identities[x.ID] {
			return problem(providers.ErrorUnavailable, "get_server", "INVENTORY_DISAGREEMENT", 200)
		}
	}
	if len(basic) != len(detailed) {
		return problem(providers.ErrorUnavailable, "get_server", "INVENTORY_DISAGREEMENT", 200)
	}
	if err = providers.ConfirmAbsence(ctx, d.absence, id); err != nil {
		return err
	}
	return problem(providers.ErrorNotFound, "get_server", "SERVER_ABSENT", 404)
}
func (d *Driver) absenceDisagreement(ctx context.Context, id string) error {
	if d.absence != nil {
		if err := d.absence.Present(ctx, id); err != nil {
			return err
		}
	}
	return problem(providers.ErrorUnavailable, "get_server", "INVENTORY_DISAGREEMENT", 200)
}
