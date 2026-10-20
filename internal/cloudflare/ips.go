package cloudflare

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"time"
)

const OfficialIPSource = "https://api.cloudflare.com/client/v4/ips"

type IPSource interface {
	Ranges(context.Context) ([]netip.Prefix, error)
}
type OfficialRanges struct{}

func (OfficialRanges) Ranges(ctx context.Context) ([]netip.Prefix, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", OfficialIPSource, nil)
	if e != nil {
		return nil, e
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	if e != nil {
		return nil, errors.New("cloudflare_ip_source_unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New("cloudflare_ip_source_unavailable")
	}
	var env struct {
		Success bool `json:"success"`
		Result  struct {
			V4 []string `json:"ipv4_cidrs"`
			V6 []string `json:"ipv6_cidrs"`
		} `json:"result"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&env) != nil || !env.Success || len(env.Result.V4) == 0 {
		return nil, errors.New("cloudflare_ip_source_invalid")
	}
	out := []netip.Prefix{}
	for _, s := range append(env.Result.V4, env.Result.V6...) {
		p, e := netip.ParsePrefix(s)
		if e != nil || !p.Addr().IsGlobalUnicast() || p.Addr().IsPrivate() || p.Bits() < 8 {
			return nil, errors.New("cloudflare_ip_source_invalid")
		}
		out = append(out, p.Masked())
	}
	return out, nil
}
func (s Service) RefreshRanges(ctx context.Context, source IPSource) error {
	return s.Locked(ctx, "ip-ranges", func() error {
		ranges, e := source.Ranges(ctx)
		if e != nil {
			return e
		}
		if len(ranges) < 1 || len(ranges) > 1000 {
			return errors.New("cloudflare_ip_source_invalid")
		}
		tx, e := s.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if _, e = tx.ExecContext(ctx, "DELETE FROM cloudflare_ip_ranges"); e != nil {
			return e
		}
		for _, p := range ranges {
			if _, e = tx.ExecContext(ctx, "INSERT INTO cloudflare_ip_ranges(network) VALUES($1)", p.String()); e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, "UPDATE cloudflare_ip_refresh SET updated_at=now(),attempted_at=now() WHERE singleton"); e != nil {
			return e
		}
		return tx.Commit()
	})
}
func (s Service) RefreshRangesIfDue(ctx context.Context) error {
	var due bool
	e := s.DB.QueryRowContext(ctx, `WITH claimed AS(UPDATE cloudflare_ip_refresh SET attempted_at=now() WHERE singleton AND (updated_at IS NULL OR updated_at<now()-interval '1 day') AND (attempted_at IS NULL OR attempted_at<now()-interval '15 minutes') RETURNING true) SELECT EXISTS(SELECT 1 FROM claimed)`).Scan(&due)
	if e != nil || !due {
		return e
	}
	return s.RefreshRanges(ctx, OfficialRanges{})
}
func ValidOperator(s string) bool {
	switch s {
	case "", "mci", "irancell", "rightel", "mokhaberat", "shatel", "other":
		return true
	}
	return false
}

type Measurement struct {
	ID               string    `json:"id"`
	RecordID         string    `json:"record_id"`
	Operator         string    `json:"operator"`
	IP               string    `json:"ip"`
	LatencyMS        int       `json:"latency_ms"`
	MeasuredAt       time.Time `json:"measured_at"`
	ProtocolVerified bool      `json:"protocol_verified"`
}

// Only authenticated administrators can submit results from their operator probe.
// These are explicitly operator-reported; datacenter probes never label an ISP.
func (s Service) SaveMeasurement(ctx context.Context, m Measurement) error {
	ip, e := netip.ParseAddr(m.IP)
	if e != nil || !ValidID(m.ID) || !ValidID(m.RecordID) || m.Operator == "" || !ValidOperator(m.Operator) || !m.ProtocolVerified || m.LatencyMS < 1 || m.LatencyMS > 60000 || m.MeasuredAt.After(time.Now().Add(time.Minute)) || m.MeasuredAt.Before(time.Now().Add(-time.Hour)) {
		return errors.New("cloudflare_measurement_invalid")
	}
	var valid bool
	var account string
	e = s.DB.QueryRowContext(ctx, `SELECT r.account_id::text,EXISTS(SELECT 1 FROM cloudflare_ip_ranges WHERE $2::inet<<=network AND fetched_at>now()-interval '7 days') FROM cloudflare_records r JOIN cloudflare_accounts a ON a.id=r.account_id WHERE r.id=$1 AND a.enabled AND r.state='DNS_READY' AND r.expires_at>now()`, m.RecordID, ip.String()).Scan(&account, &valid)
	if e != nil || !valid {
		return errors.New("cloudflare_measurement_not_owned")
	}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO cloudflare_ip_measurements(id,account_id,record_id,operator,ip,latency_ms,measured_at,source) VALUES($1,$2,$3,$4,$5,$6,$7,'operator_probe') ON CONFLICT(id) DO NOTHING`, m.ID, account, m.RecordID, m.Operator, ip.String(), m.LatencyMS, m.MeasuredAt)
	return e
}
func selectEdge(ctx context.Context, tx *sql.Tx, account, zone, operator, hostname string) (string, error) {
	if operator == "" {
		return hostname, nil
	}
	var ip string
	e := tx.QueryRowContext(ctx, `SELECT host(m.ip) FROM cloudflare_ip_measurements m JOIN cloudflare_records r ON r.id=m.record_id
 WHERE m.account_id=$1 AND r.zone_id=$2 AND m.operator=$3 AND m.measured_at>now()-interval '1 hour'
 AND EXISTS(SELECT 1 FROM cloudflare_ip_ranges p WHERE m.ip<<=p.network AND p.fetched_at>now()-interval '7 days')
 ORDER BY m.latency_ms,m.measured_at DESC LIMIT 1`, account, zone, operator).Scan(&ip)
	if errors.Is(e, sql.ErrNoRows) {
		return hostname, nil
	}
	return ip, e
}
