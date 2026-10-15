package transportprofile

import (
	"context"
	"database/sql"
	"encoding/json"
)

func PlannedFlow(ctx context.Context, db *sql.DB, panel string, id int64, network string) (string, error) {
	if network == "tcp" {
		return Vision, nil
	}
	if network != "xhttp" {
		return "", ErrInvalid
	}
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT COALESCE(p.config,'{}'::jsonb) FROM panel_inbound_inventory i LEFT JOIN reality_listener_profiles p ON p.panel_id=i.panel_id AND p.managed_key=i.remark WHERE i.panel_id=$1 AND i.remote_id=$2 AND i.present AND i.enabled`, panel, id).Scan(&raw)
	if err != nil {
		return "", err
	}
	var c Config
	if json.Unmarshal(raw, &c) != nil {
		return "", ErrInvalid
	}
	// Unpinned historical XHTTP listeners had empty flow. The worker verifies
	// this against a fresh native snapshot before sending a client mutation.
	if c.Network == "" {
		return "", nil
	}
	if c.NetworkName() != network || c.Validate(true) != nil {
		return "", ErrPinned
	}
	return c.ServerFlow(), nil
}

// ApplyPinnedClientOptions only overlays client-specific settings after the
// observed server contract matches the immutable pin. It never uses defaults.
func ApplyPinnedClientOptions(observed, pin Config) (Config, error) {
	if pin.Validate(pin.NetworkName() == "xhttp") != nil || observed.NetworkName() != pin.NetworkName() {
		return Config{}, ErrPinned
	}
	if pin.NetworkName() == "tcp" {
		return observed, nil
	}
	if observed.Host != pin.Host || observed.Path != pin.Path || observed.Mode != pin.Mode {
		return Config{}, ErrPinned
	}
	a, _ := json.Marshal(observed.WireSettings())
	b, _ := json.Marshal(pin.WireSettings())
	if !equalJSON(a, b) {
		return Config{}, ErrPinned
	}
	if observed.MaskOnServer() != pin.MaskOnServer() {
		return Config{}, ErrPinned
	}
	if pin.MaskOnServer() {
		a, e := observed.TCPMask()
		if e != nil {
			return Config{}, e
		}
		b, e := pin.TCPMask()
		if e != nil || !equalJSON(a, b) {
			return Config{}, ErrPinned
		}
	}
	return pin, nil
}

// ObservedClientFlow translates a verified wire flow into its client option.
// Missing or drifted native flow is never repaired from a saved profile.
func ObservedClientFlow(c Config, observed string) (string, error) {
	if c.NetworkName() != "xhttp" {
		return observed, nil
	}
	if observed != c.ServerFlow() {
		return "", ErrPinned
	}
	return c.Flow, nil
}
