package configplane

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

// Busy panels yield their slot immediately. A stalled server cannot consume a
// worker merely by waiting for another operation on that same server.
func (s Service) withPanelLock(ctx context.Context, panel string, fn func(context.Context) error) error {
	held, e := sanaei.TryWithConfigLock(ctx, s.DB, panel, fn)
	if e != nil {
		return e
	}
	if !held {
		return errors.New("profile_panel_busy")
	}
	return nil
}
