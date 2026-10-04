package usercapacity

import (
	"context"
	"database/sql"
	"math"
	"time"
)

func (s Service) durableAllowance(ctx context.Context, panelID string, inboundID int64, rate, deficit int, now time.Time, classes ...string) (int, error) {
	if s.DB == nil || panelID == "" || inboundID <= 0 || rate <= 0 || deficit <= 0 {
		return 0, nil
	}
	class := ""
	if len(classes) > 0 {
		class = classes[0]
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
INSERT INTO bulk_user_rate_state(panel_id,inbound_id,tokens,last_refill_at,updated_at,route_class)
VALUES($1,$2,0,$3,$3,$4)
ON CONFLICT(panel_id,inbound_id,route_class) DO NOTHING
`, panelID, inboundID, now, class)
	if err != nil {
		return 0, err
	}
	var tokens float64
	var last time.Time
	if err = tx.QueryRowContext(ctx, `
SELECT tokens,last_refill_at FROM bulk_user_rate_state
WHERE panel_id=$1 AND inbound_id=$2 AND route_class=$3
FOR UPDATE
`, panelID, inboundID, class).Scan(&tokens, &last); err != nil {
		return 0, err
	}
	tokens, n := durableRateStep(tokens, last, rate, deficit, now)
	_, err = tx.ExecContext(ctx, `
UPDATE bulk_user_rate_state SET tokens=$3,last_refill_at=$4,updated_at=now()
WHERE panel_id=$1 AND inbound_id=$2 AND route_class=$5
`, panelID, inboundID, tokens, now, class)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

func durableRateStep(tokens float64, last time.Time, rate, deficit int, now time.Time) (float64, int) {
	elapsed := now.Sub(last).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	tokens += elapsed * float64(rate)
	if tokens > float64(rate) {
		tokens = float64(rate)
	}
	n := int(math.Floor(tokens))
	if n > deficit {
		n = deficit
	}
	tokens -= float64(n)
	return tokens, n
}

// BulkAllowance shares the production durable token bucket with scoped canaries.
func (s Service) BulkAllowance(ctx context.Context, panelID string, inboundID int64, rate, deficit int) (int, error) {
	if rate < 1 || rate > 100 || deficit < 1 || deficit > 250 {
		return 0, nil
	}
	return s.durableAllowance(ctx, panelID, inboundID, rate, deficit, time.Now())
}
