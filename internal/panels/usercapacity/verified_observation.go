package usercapacity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportprofile"
)

// ObserveVerifiedSnapshot reuses a fresh complete native inventory already read
// by another panel owner. It performs no native calls or client mutations.
// Legacy unmanaged inbounds retain their existing observer. Missing required
// traffic leaves the previous capacity evidence untouched, not freshly zeroed.
func (s Service) ObserveVerifiedSnapshot(ctx context.Context, panel string, raws []json.RawMessage) error {
	return observeVerifiedSnapshot(ctx, s.DB, panel, raws)
}
func (s Service) ObserveVerifiedSnapshotTx(ctx context.Context, tx *sql.Tx, panel string, raws []json.RawMessage) error {
	return observeVerifiedSnapshot(ctx, tx, panel, raws)
}
func observeVerifiedSnapshot(ctx context.Context, db observationDB, panel string, raws []json.RawMessage) error {
	for _, raw := range raws {
		var in rawInbound
		if err := json.Unmarshal(raw, &in); err != nil {
			return err
		}
		if !in.Enable || in.Protocol != "vless" {
			continue
		}
		var stream struct {
			Network  string `json:"network"`
			Security string `json:"security"`
		}
		streamRaw := in.StreamSettings
		var encoded string
		if json.Unmarshal(streamRaw, &encoded) == nil {
			streamRaw = []byte(encoded)
		}
		if json.Unmarshal(streamRaw, &stream) != nil || !transportprofile.Supported(stream.Network) || stream.Security != "reality" {
			continue
		}
		var bulk bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM bulk_user_ownership o JOIN bulk_user_generations g ON g.id=o.generation_id WHERE g.panel_id=$1 AND g.inbound_id=$2 AND o.mutation_job_id IS NOT NULL AND o.state IN ('PLANNED','ACTIVE','DELETE_PENDING'))`, panel, in.ID).Scan(&bulk); err != nil {
			return err
		}
		if !bulk {
			continue
		}
		if err := observeBulkCapacity(ctx, db, panel, in); err != nil && !errors.Is(err, clientops.ErrVerify) {
			return err
		}
	}
	return nil
}
