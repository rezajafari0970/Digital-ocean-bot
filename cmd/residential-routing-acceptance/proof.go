package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

// The snapshot and route matrix share the existing cross-process panel lock.
// Concurrent Sanaei TestRoute handlers share a mutable gRPC client; accepting
// without this barrier can race the worker's own route verification or updates.
func verifyUnderConfigLock(ctx context.Context, db *sql.DB, panel string, verify func(context.Context) (int, error)) (int, error) {
	count := 0
	err := sanaei.WithConfigLock(ctx, db, panel, func(locked context.Context) error {
		var err error
		count, err = verify(locked)
		if err != nil && locked.Err() == nil {
			count, err = verify(locked)
		}
		return err
	})
	return count, err
}

// A failed or malformed API envelope is unavailable evidence, not a routing
// mismatch. Only a successful runtime response can establish the latter.
func routeProofMatches(resp sanaei.SessionResponse, expected string) (bool, error) {
	var result struct {
		Success bool
		Obj     struct {
			Matched     bool
			OutboundTag string
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || json.Unmarshal(resp.Body, &result) != nil || !result.Success {
		return false, errors.New("route API result unavailable")
	}
	return result.Obj.Matched && result.Obj.OutboundTag == expected, nil
}

// A non-pool HTTP(S) endpoint cannot carry protected UDP. Its preceding
// explicit deny rule, rather than the later shared ads rule, is authoritative.
func protectedForNetwork(network, protected, udpDeny string) string {
	if network == "udp" && udpDeny != "" {
		return udpDeny
	}
	return protected
}
