package usercapacity

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/clientops"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"time"
)

func (s Service) reconcileLifecycle(ctx context.Context, p readyworker.Panel, rt *sanaei.PanelRuntime) (bool, bool, error) {
	if s.DB == nil {
		return false, false, errors.New("user capacity config")
	}
	var enabled bool
	if err := s.DB.QueryRowContext(ctx, `SELECT enabled FROM bulk_lifecycle_control WHERE singleton`).Scan(&enabled); err != nil {
		return true, false, err
	}
	if !enabled {
		// Once this version is installed, a closed lifecycle control must not
		// fall through into direct legacy mutations. Compatibility code stays
		// available for explicitly selected legacy paths.
		return true, false, nil
	}
	if rt == nil || rt.Session == nil {
		return true, false, errors.New("user capacity runtime")
	}
	if err := s.autoEnrollLifecycle(ctx, p, rt); err != nil {
		return true, false, err
	}
	j := clientops.Journal{DB: s.DB}
	ids, err := j.LifecycleInbounds(ctx, p.ID)
	if err != nil {
		return true, false, err
	}
	planned := false
	if len(ids) == 0 {
		return true, false, nil
	}
	err = rt.WithMutation(ctx, func(c context.Context) error {
		for _, id := range ids {
			did, e := j.PlanLifecycle(c, rt, id, func(c context.Context, rate, n int) (int, error) {
				return s.durableAllowance(c, p.ID, id, rate, n, time.Now(), clientops.ProfileRateClass(c))
			})
			if e != nil {
				reportCtx, reportCancel := context.WithTimeout(context.WithoutCancel(c), 5*time.Second)
				_, _ = s.DB.ExecContext(reportCtx, `UPDATE bulk_lifecycle_scopes SET last_error=$3,updated_at=now() WHERE panel_id=$1 AND inbound_id=$2`, p.ID, id, e.Error())
				reportCancel()
				return e
			}
			planned = planned || did
		}
		return nil
	})
	return true, planned, err
}
