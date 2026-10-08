package network

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"log"
	"time"
)

type Monitor struct {
	DB        *sql.DB
	Secrets   *secrets.Store
	Interval  time.Duration
	Timeout   time.Duration
	Policy    HealthPolicy
	Endpoint  string
	Economy   *EconomyController
	revisions map[string]string
}

func (m Monitor) Run(ctx context.Context) error {
	interval := m.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	m.revisions = make(map[string]string)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		supervision.Pulse(ctx)
		m.runOnce(ctx)
		supervision.Idle(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func (m Monitor) runOnce(ctx context.Context) {
	rows, err := m.DB.QueryContext(ctx, `SELECT p.id::text,p.name,p.type,p.host,p.port,COALESCE(p.username,''),COALESCE(p.secret_ref,''),p.status,COALESCE(p.exit_ip::text,''),p.failure_count,p.consecutive_successes,p.last_checked_at,p.last_success_at,COALESCE(p.health_error,''),COALESCE((SELECT max(updated_at)::text FROM secrets WHERE proxy_id=p.id),'') FROM proxies p`)
	if err != nil {
		log.Printf("proxy monitor query: %v", err)
		return
	}
	defer rows.Close()
	type item struct {
		p                                     Proxy
		user, ref, diagnostic, secretRevision string
		checked, success                      sql.NullTime
		successes                             int
	}
	var list []item
	for rows.Next() {
		var x item
		if rows.Scan(&x.p.ID, &x.p.Name, &x.p.Type, &x.p.Host, &x.p.Port, &x.user, &x.ref, &x.p.Status, &x.p.ExitIP, &x.p.FailureCount, &x.successes, &x.checked, &x.success, &x.diagnostic, &x.secretRevision) == nil {
			list = append(list, x)
		}
	}
	rows.Close()
	economy, _ := m.Economy.Enabled(ctx, "")
	for _, x := range list {
		revision := string(x.p.Type) + "/" + x.p.Host + "/" + fmt.Sprint(x.p.Port) + "/" + x.user + "/" + x.ref + "/" + x.secretRevision
		unchanged := m.revisions != nil && m.revisions[x.p.ID] == revision
		if m.revisions != nil {
			m.revisions[x.p.ID] = revision
		}
		if economy && unchanged && baseProbeMayWait(x.p.Status, x.p.FailureCount, x.successes, x.diagnostic, x.checked.Time, time.Now()) {
			continue
		}
		supervision.Pulse(ctx)
		password := []byte(nil)
		if x.ref != "" {
			password, err = m.Secrets.GetProxy(ctx, x.p.ID, x.ref)
			if err != nil {
				m.markFailure(ctx, x)
				continue
			}
		}
		state := HealthState{Status: x.p.Status, ConsecutiveFailures: x.p.FailureCount, ConsecutiveSuccesses: x.successes, LastExitIP: x.p.ExitIP}
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
		_, err = scheduler.Check(ctx, "proxy-monitor:"+x.p.ID, HealthTarget{Proxy: probe, Credentials: ProxyCredentials{Username: x.user, Password: string(password)}, ExpectedExitIP: "", Endpoint: endpoint, State: state})
		wipeBytes(password)
		if err != nil {
			log.Printf("proxy monitor %s: %v", x.p.Name, err)
		}
	}
}
func (m Monitor) markFailure(ctx context.Context, x struct {
	p                                     Proxy
	user, ref, diagnostic, secretRevision string
	checked, success                      sql.NullTime
	successes                             int
}) {
	state := HealthState{Status: x.p.Status, ConsecutiveFailures: x.p.FailureCount, ConsecutiveSuccesses: x.successes, LastExitIP: x.p.ExitIP, LastCheckedAt: time.Now().UTC()}
	state = state.Apply(HealthResult{Status: StatusDown, CheckedAt: time.Now().UTC(), Error: "secret unavailable"}, m.Policy)
	_ = (SQLHealthStore{DB: m.DB}).Save(ctx, x.p.ID, state)
}
func wipeBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func baseProbeMayWait(status ProxyStatus, failures, successes int, diagnostic string, checked, now time.Time) bool {
	if checked.IsZero() || checked.After(now) || now.Sub(checked) >= EconomyBaseInterval {
		return false
	}
	if status == StatusHealthy && failures == 0 && successes >= 2 {
		return true
	}
	return status == StatusDown && failures >= 2 && (diagnostic == "PROXY_AUTH_FAILED" || diagnostic == "PROXY_AUTH_METHOD_UNSUPPORTED")
}
