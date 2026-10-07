package residential

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
	"net"
	"strings"
	"sync"
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
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	started := time.Now()
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
	failure := "Endpoint connectivity check failed"
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
			if strings.Contains(strings.ToLower(observed.Error), "auth") {
				failure = "Proxy authentication rejected; check credentials or provider access"
			} else if strings.Contains(strings.ToLower(observed.Error), "resolve") || strings.Contains(strings.ToLower(observed.Error), "no such host") {
				failure = "Proxy DNS resolution failed"
			} else if strings.Contains(strings.ToLower(observed.Error), "timeout") || strings.Contains(strings.ToLower(observed.Error), "deadline") {
				failure = "Proxy check timed out"
			}
		}
	}
	if parent.Err() != nil {
		return Result{}, parent.Err()
	}
	saveCtx, stop := context.WithTimeout(context.WithoutCancel(parent), 3*time.Second)
	defer stop()
	res, err := m.DB.ExecContext(saveCtx, `UPDATE residential_proxies SET status=$2,exit_ip=NULLIF($3,'')::inet,latency_ms=$4::bigint,
 last_checked_at=now(),last_success_at=CASE WHEN $2='healthy' THEN now() ELSE last_success_at END,
 last_error=CASE WHEN $2='healthy' THEN '' ELSE $7 END,
 success_ewma=CASE WHEN check_count=0 THEN CASE WHEN $2='healthy' THEN 1 ELSE 0 END ELSE success_ewma*0.8+CASE WHEN $2='healthy' THEN 0.2 ELSE 0 END END,
 latency_ewma_ms=CASE WHEN $2<>'healthy' THEN latency_ewma_ms WHEN latency_ewma_ms=0 THEN $4::bigint ELSE latency_ewma_ms*0.8+($4::bigint)*0.2 END,check_count=check_count+1
 WHERE proxy_id=$1 AND updated_at=$5 AND (last_checked_at IS NULL OR last_checked_at<=$6)`, id, result.Status, result.ExitIP, result.LatencyMS, version, started, failure)
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
		supervision.Pulse(ctx)
		rows, err := m.DB.QueryContext(ctx, `SELECT proxy_id::text FROM residential_proxies WHERE enabled AND (last_checked_at IS NULL OR last_checked_at<now()-CASE WHEN status='healthy' THEN interval '30 seconds' ELSE interval '10 seconds' END) ORDER BY last_checked_at NULLS FIRST LIMIT 256`)
		if err == nil {
			var ids []string
			for rows.Next() {
				var id string
				if rows.Scan(&id) == nil {
					ids = append(ids, id)
				}
			}
			rows.Close()
			var wg sync.WaitGroup
			jobs := make(chan string)
			for i := 0; i < 16; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for id := range jobs {
						if ctx.Err() == nil {
							_ = supervision.Work(ctx, func(ctx context.Context) error { _, err := m.Check(ctx, id); return err })
						}
					}
				}()
			}
			for _, id := range ids {
				select {
				case jobs <- id:
				case <-ctx.Done():
				}
			}
			close(jobs)
			wg.Wait()

		}
		supervision.Idle(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}
