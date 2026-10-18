package configplane

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

// Removed identities remain journaled until the panel's global user store also
// confirms their absence. A restart cannot forget unfinished shrink cleanup.
func (s Service) retireUnusedClients(ctx context.Context, rt *sanaei.PanelRuntime, id, panel string, raws []json.RawMessage) error {
	rows, e := s.DB.QueryContext(ctx, "SELECT client_id,email FROM config_driver_client_retirements WHERE profile_id=$1 AND panel_id=$2 AND confirmed_absent_at IS NULL ORDER BY client_id LIMIT 200", id, panel)
	if e != nil {
		return e
	}
	owned := map[string]string{}
	ids := []string{}
	for rows.Next() {
		var client, email string
		if e = rows.Scan(&client, &email); e != nil {
			break
		}
		owned[email] = client
		ids = append(ids, client)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return e
	}
	if len(ids) == 0 {
		return nil
	}
	for _, b := range raws {
		var x struct{ Settings json.RawMessage }
		if json.Unmarshal(b, &x) != nil {
			return errors.New("profile_inventory_invalid")
		}
		var text string
		if json.Unmarshal(x.Settings, &text) == nil {
			x.Settings = []byte(text)
		}
		var settings struct{ Clients []struct{ Email string } }
		if json.Unmarshal(x.Settings, &settings) != nil {
			return errors.New("profile_inventory_invalid")
		}
		for _, c := range settings.Clients {
			if owned[c.Email] != "" {
				return errors.New("profile_retired_client_still_attached")
			}
		}
	}
	users, e := sanaei.ReadGlobalClientsSession(ctx, rt.Session.Exec)
	if e != nil {
		return e
	}
	emails := []string{}
	for _, c := range users {
		if expected := owned[c.Email]; expected != "" {
			if expected != c.UUID {
				return errors.New("profile_retired_client_identity_changed")
			}
			emails = append(emails, c.Email)
		}
	}
	for len(emails) > 0 {
		n := len(emails)
		if n > 100 {
			n = 100
		}
		_, _ = sanaei.BulkDeleteClientsSession(ctx, rt.Session.Exec, emails[:n])
		emails = emails[n:]
	}
	if len(owned) > 0 {
		users, e = sanaei.ReadGlobalClientsSession(ctx, rt.Session.Exec)
		if e != nil {
			return e
		}
		for _, c := range users {
			if owned[c.Email] != "" {
				return errors.New("profile_retired_client_delete_unverified")
			}
		}
	}
	_, e = s.DB.ExecContext(ctx, "UPDATE config_driver_client_retirements SET confirmed_absent_at=now() WHERE profile_id=$1 AND panel_id=$2 AND client_id=ANY($3::text[])", id, panel, pq.Array(ids))
	return e
}
