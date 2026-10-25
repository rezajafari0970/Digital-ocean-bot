package configplane

import (
	"context"
	"errors"
)

// A deleted profile relinquishes its fleet-wide desired port, not the actual
// sockets or ambiguous writes it still owns. These reservations block only the
// affected panel. Call again under the panel writer lock before remote writes;
// SENT recovery remains observation-only and must not be suppressed by this gate.
func (s Service) checkListenerPortReservation(ctx context.Context, profile, panel string, port int) error {
	var occupied, cleanupOnly bool
	err := s.DB.QueryRowContext(ctx, `SELECT count(*)>0,COALESCE(bool_and(owner.delete_requested_at IS NOT NULL),false)
 FROM config_driver_listeners l JOIN config_driver_profiles owner ON owner.id=l.profile_id
 WHERE l.panel_id=$2 AND l.profile_id<>$1 AND l.state<>'RETIRED'
 AND ((l.remote_id IS NOT NULL AND (l.applied_spec->>'port')::integer=$3)
 OR (l.mutation_state IN('PREPARED','SENT') AND (l.mutation_spec->>'port')::integer=$3))`, profile, panel, port).Scan(&occupied, &cleanupOnly)
	if err != nil {
		return err
	}
	if occupied {
		if cleanupOnly {
			return errors.New("profile_port_cleanup_pending")
		}
		return errors.New("profile_port_conflict")
	}
	return nil
}
