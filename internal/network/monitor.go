package network

import (
	"context"
	"database/sql"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"log"
	"time"
)

type Monitor struct {
	DB       *sql.DB
	Secrets  *secrets.Store
	Interval time.Duration
	Timeout  time.Duration
	Policy   HealthPolicy
	Endpoint string
}

func (m Monitor) Run(ctx context.Context) error {
	interval := m.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		m.runOnce(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func (m Monitor) runOnce(ctx context.Context) {
	rows, err := m.DB.QueryContext(ctx, `SELECT id::text,name,type,host,port,COALESCE(username,''),COALESCE(secret_ref,''),status,COALESCE(exit_ip::text,''),failure_count,last_checked_at,last_success_at FROM proxies`)
	if err != nil {
		log.Printf("proxy monitor query: %v", err)
		return
	}
	defer rows.Close()
	type item struct {
		p                Proxy
		user, ref        string
		checked, success sql.NullTime
	}
	var list []item
	for rows.Next() {
		var x item
		if rows.Scan(&x.p.ID, &x.p.Name, &x.p.Type, &x.p.Host, &x.p.Port, &x.user, &x.ref, &x.p.Status, &x.p.ExitIP, &x.p.FailureCount, &x.checked, &x.success) == nil {
			list = append(list, x)
		}
	}
	rows.Close()
	for _, x := range list {
		password := []byte(nil)
		if x.ref != "" {
			password, err = m.Secrets.GetProxy(ctx, x.p.ID, x.ref)
			if err != nil {
				m.markFailure(ctx, x)
				continue
			}
		}
		state := HealthState{Status: x.p.Status, ConsecutiveFailures: x.p.FailureCount, LastExitIP: x.p.ExitIP}
		if x.checked.Valid {
			state.LastCheckedAt = x.checked.Time
		}
		if x.success.Valid {
			state.LastSuccessAt = x.success.Time
		}
		endpoint := m.Endpoint
		if endpoint == "" {
			endpoint = "https://api.ipify.org?format=json"
		}
		probe := x.p
		probe.Status = StatusHealthy
		scheduler := HealthScheduler{Store: SQLHealthStore{DB: m.DB}, Policy: m.Policy, Timeout: m.Timeout}
		_, err = scheduler.Check(ctx, "proxy-monitor:"+x.p.ID, HealthTarget{Proxy: probe, Credentials: ProxyCredentials{Username: x.user, Password: string(password)}, ExpectedExitIP: x.p.ExitIP, Endpoint: endpoint, State: state})
		wipeBytes(password)
		if err != nil {
			log.Printf("proxy monitor %s: %v", x.p.Name, err)
		}
	}
}
func (m Monitor) markFailure(ctx context.Context, x struct {
	p                Proxy
	user, ref        string
	checked, success sql.NullTime
}) {
	state := HealthState{Status: x.p.Status, ConsecutiveFailures: x.p.FailureCount, LastExitIP: x.p.ExitIP, LastCheckedAt: time.Now().UTC()}
	state = state.Apply(HealthResult{Status: StatusDown, CheckedAt: time.Now().UTC(), Error: "secret unavailable"}, m.Policy)
	_ = (SQLHealthStore{DB: m.DB}).Save(ctx, x.p.ID, state)
}
func wipeBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
