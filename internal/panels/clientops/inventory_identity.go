package clientops

import "github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"

// Native v3 can attach one global client to several inbounds. That is valid for
// unrelated clients, but duplicate records within an inbound or inconsistent
// ID/email mappings anywhere are not authoritative inventory.
type inventoryIdentities struct {
	ids, emails           map[string]string
	localIDs, localEmails map[int64]map[string]bool
}

func (v *inventoryIdentities) check(inbound int64, c sanaei.Client) error {
	if inbound <= 0 || c.ID == "" || c.Email == "" {
		return sanaei.ErrInventoryRejected
	}
	if v.ids == nil {
		v.ids = map[string]string{}
		v.emails = map[string]string{}
		v.localIDs = map[int64]map[string]bool{}
		v.localEmails = map[int64]map[string]bool{}
	}
	if v.localIDs[inbound] == nil {
		v.localIDs[inbound] = map[string]bool{}
		v.localEmails[inbound] = map[string]bool{}
	}
	if v.localIDs[inbound][c.ID] || v.localEmails[inbound][c.Email] {
		return sanaei.ErrInventoryRejected
	}
	if email, ok := v.ids[c.ID]; ok && email != c.Email {
		return sanaei.ErrInventoryRejected
	}
	if id, ok := v.emails[c.Email]; ok && id != c.ID {
		return sanaei.ErrInventoryRejected
	}
	v.localIDs[inbound][c.ID] = true
	v.localEmails[inbound][c.Email] = true
	v.ids[c.ID] = c.Email
	v.emails[c.Email] = c.ID
	return nil
}
