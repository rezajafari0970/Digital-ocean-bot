package configplane

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"time"
)

func nextProfileCheck(p Profile, c configdriver.Credentials, now time.Time) time.Time {
	next := now.Add(2 * time.Minute)
	counts := map[string]int{}
	for _, cl := range c.Clients {
		if configdriver.Active(cl, now) {
			counts[cl.Class]++
		}
		if cl.ExpiresAt > 0 {
			expiry := time.UnixMilli(cl.ExpiresAt)
			if expiry.Before(next) {
				next = expiry
			}
		}
	}
	for _, class := range p.Classes {
		policy := p.ClassSettings[class]
		if counts[class] < policy.Count {
			delay := policy.CreationIntervalSeconds
			if policy.UsersPerSecond > 0 && delay < 1 {
				delay = 1
			}
			due := time.UnixMilli(c.LastCreated[class]).Add(time.Duration(delay) * time.Second)
			if due.Before(next) {
				next = due
			}
		}
	}
	if next.Before(now.Add(time.Second)) {
		next = now.Add(time.Second)
	}
	return next
}
