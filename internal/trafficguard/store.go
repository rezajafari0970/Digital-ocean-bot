package trafficguard

import (
	"context"
	"database/sql"
	"errors"
)

var ErrNoSample = errors.New("no traffic sample")

type Store interface {
	LastSample(context.Context, string) (Sample, error)
	SaveSample(context.Context, Sample) error
	LoadBaseline(context.Context, string) (Baseline, error)
	SaveBaseline(context.Context, Baseline) error
	SaveDecision(context.Context, string, Decision) error
}
type SQLStore struct{ DB *sql.DB }

func (s SQLStore) LastSample(ctx context.Context, id string) (Sample, error) {
	var x Sample
	x.ClientID = id
	err := s.DB.QueryRowContext(ctx, `SELECT up_bytes,down_bytes,sampled_at FROM xui_traffic_samples WHERE client_id=$1 ORDER BY sampled_at DESC LIMIT 1`, id).Scan(&x.UpBytes, &x.DownBytes, &x.At)
	if errors.Is(err, sql.ErrNoRows) {
		return Sample{}, ErrNoSample
	}
	return x, err
}
func (s SQLStore) SaveSample(ctx context.Context, x Sample) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO xui_traffic_samples(client_id,up_bytes,down_bytes,sampled_at) VALUES($1,$2,$3,$4)`, x.ClientID, x.UpBytes, x.DownBytes, x.At)
	return err
}
func (s SQLStore) LoadBaseline(ctx context.Context, id string) (Baseline, error) {
	var b Baseline
	b.ClientID = id
	err := s.DB.QueryRowContext(ctx, `SELECT ewma_bps,variance,samples,consecutive_anomalies,updated_at FROM traffic_baselines WHERE client_id=$1`, id).Scan(&b.EWMABytesPerSec, &b.Variance, &b.Samples, &b.ConsecutiveAnomalies, &b.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return b, nil
	}
	return b, err
}

func (s SQLStore) SaveBaseline(ctx context.Context, b Baseline) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO traffic_baselines(client_id,ewma_bps,variance,samples,consecutive_anomalies,updated_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(client_id) DO UPDATE SET ewma_bps=EXCLUDED.ewma_bps,variance=EXCLUDED.variance,samples=EXCLUDED.samples,consecutive_anomalies=EXCLUDED.consecutive_anomalies,updated_at=EXCLUDED.updated_at`, b.ClientID, b.EWMABytesPerSec, b.Variance, b.Samples, b.ConsecutiveAnomalies, b.UpdatedAt)
	return err
}
func (s SQLStore) SaveDecision(ctx context.Context, id string, d Decision) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO traffic_events(client_id,suspicious,confirmed,rate_bps,threshold_bps,reason,action) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, d.Suspicious, d.Confirmed, d.RateBytesPerSec, d.Threshold, d.Reason, d.Action)
	return err
}
