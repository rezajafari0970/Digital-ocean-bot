package residential

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"net"
	"time"
)

type Secrets interface {
	GetResidential(context.Context, string, string) ([]byte, error)
}
type Monitor struct {
	DB       *sql.DB
	Secrets  Secrets
	Endpoint string
}
type Result struct {
	Status    string `json:"status"`
	ExitIP    string `json:"exit_ip"`
	LatencyMS int64  `json:"latency_ms"`
}

// Only endpoint connectivity is checked. No account identity or stickiness rules apply.
func (m Monitor) Check(parent context.Context, id string) (Result, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	var p network.Proxy
	var user, ref string
	var version time.Time
	err := m.DB.QueryRowContext(ctx, `SELECT type,host,port,COALESCE(username,''),COALESCE(secret_ref,''),updated_at FROM residential_proxies WHERE proxy_id=$1`, id).Scan(&p.Type, &p.Host, &p.Port, &user, &ref, &version)
	if err != nil {
		return Result{}, err
	}
	p.ID = id
	p.Status = network.StatusHealthy
	var secret []byte
	if ref != "" {
		secret, err = m.Secrets.GetResidential(ctx, id, ref)
	}
	defer func() {
		for i := range secret {
			secret[i] = 0
		}
	}()
	result := Result{Status: "down"}
	if err == nil {
		gateway, e := network.NewProxyProbeGateway("residential:"+id, p, network.ProxyCredentials{Username: user, Password: string(secret)})
		if e == nil {
			defer gateway.CloseIdleConnections()
			endpoint := m.Endpoint
			if endpoint == "" {
				endpoint = "https://api.ipify.org?format=json"
			}
			observed := network.CheckProxy(ctx, gateway, endpoint, "", "")
			if observed.Status == network.StatusHealthy && net.ParseIP(observed.ExitIP) != nil {
				result.Status = "healthy"
				result.ExitIP = observed.ExitIP
			}
			result.LatencyMS = observed.Latency.Milliseconds()
		}
	}
	saveCtx, stop := context.WithTimeout(context.WithoutCancel(parent), 3*time.Second)
	defer stop()
	res, err := m.DB.ExecContext(saveCtx, `UPDATE residential_proxies SET status=$2,exit_ip=NULLIF($3,'')::inet,latency_ms=$4,
 last_checked_at=now(),last_success_at=CASE WHEN $2='healthy' THEN now() ELSE last_success_at END,
 last_error=CASE WHEN $2='healthy' THEN '' ELSE 'Endpoint connectivity check failed' END
 WHERE proxy_id=$1 AND updated_at=$5`, id, result.Status, result.ExitIP, result.LatencyMS, version)
	if err != nil {
		return Result{}, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return Result{}, errors.New("residential endpoint changed during check")
	}
	return result, nil
}
func (m Monitor) Run(ctx context.Context) error {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		rows, err := m.DB.QueryContext(ctx, "SELECT proxy_id::text FROM residential_proxies WHERE enabled ORDER BY last_checked_at NULLS FIRST")
		if err == nil {
			var ids []string
			for rows.Next() {
				var id string
				if rows.Scan(&id) == nil {
					ids = append(ids, id)
				}
			}
			rows.Close()
			for _, id := range ids {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				_, _ = m.Check(ctx, id)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}
