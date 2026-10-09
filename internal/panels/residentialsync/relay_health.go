package residentialsync

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/supervision"
)

type relayObservation struct {
	Tag          string `json:"tag"`
	Alive        bool   `json:"alive"`
	Delay        int64  `json:"delay"`
	LastSeenTime int64  `json:"lastSeenTime"`
	LastTryTime  int64  `json:"lastTryTime"`
	UpdatedAt    int64  `json:"updatedAt"`
}

func decodeRelayObservations(raw []byte, now time.Time) ([]relayObservation, error) {
	var top struct {
		Success bool
		Obj     []relayObservation
	}
	if json.Unmarshal(raw, &top) != nil || !top.Success {
		return nil, errors.New("relay observation unavailable")
	}
	out := []relayObservation{}
	seen := map[string]bool{}
	for _, o := range top.Obj {
		if !strings.HasPrefix(o.Tag, "residential-ads-relay-") {
			continue
		}
		if seen[o.Tag] {
			return nil, errors.New("duplicate relay observation")
		}
		seen[o.Tag] = true
		// Burst metrics on deployed 26.9.9 have no per-attempt timestamps.
		// Alive is the core's current expiring-window result. Use sample freshness
		// and sustained bad duration; do not claim a new physical probe per poll.
		if o.LastTryTime == 0 && o.LastSeenTime == 0 {
			o.LastTryTime = o.UpdatedAt
			if o.Alive {
				o.LastSeenTime = o.UpdatedAt
			}
		}
		if o.LastTryTime <= 0 || o.LastTryTime > now.Unix()+5 || o.UpdatedAt > now.Unix()+5 || now.Unix()-o.LastTryTime > 35 || now.Unix()-o.UpdatedAt > 20 {
			continue
		}
		if o.Alive && (o.LastSeenTime <= 0 || now.Unix()-o.LastSeenTime > 35 || o.LastSeenTime > now.Unix()+5 || o.Delay < 0 || o.Delay > 3000) {
			o.Alive = false
		}
		out = append(out, o)
	}
	return out, nil
}
func (s Service) observeRelays(ctx context.Context, panel string) error {
	rt, e := s.Runtimes.Acquire(ctx, panel)
	if e != nil {
		return e
	}
	resp, e := rt.Session.Exec.Do(ctx, sanaei.SessionRequest{Method: "GET", Path: "panel/api/server/xrayObservatory", TimeoutSeconds: 5})
	if e != nil {
		return e
	}
	if resp.StatusCode != 200 {
		return errors.New("relay observation HTTP unavailable")
	}
	var now time.Time
	if e = s.DB.QueryRowContext(ctx, "SELECT clock_timestamp()").Scan(&now); e != nil {
		return e
	}
	observations, e := decodeRelayObservations(resp.Body, now)
	if e != nil {
		return e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, o := range observations {
		// Duplicate snapshots do not advance state. Quarantine requires sustained
		// unhealthy time, because burst snapshots do not expose attempt timestamps.
		_, e = tx.ExecContext(ctx, `UPDATE panel_relay_assignments a SET
   alive=$3,last_try=$4,observed_at=now(),
   last_success_at=CASE WHEN $3 THEN to_timestamp($5) ELSE a.last_success_at END,
   failures=CASE WHEN $3 THEN 0 ELSE a.failures+1 END,
   failed_since=CASE WHEN $3 THEN NULL ELSE COALESCE(a.failed_since,now()) END,
   cooldown_until=CASE WHEN NOT $3 AND a.failed_since<=now()-interval '30 seconds' THEN now()+interval '2 minutes' ELSE a.cooldown_until END
   FROM panel_routing_state r WHERE a.receiver_panel_id=$1 AND a.outbound_tag=$2 AND a.selected AND a.applied
    AND r.panel_id=a.receiver_panel_id AND r.state='APPLIED' AND r.relay_mode
    AND a.receiver_plan_hash=r.plan_hash AND a.last_try<$4`, panel, o.Tag, o.Alive, o.LastTryTime, o.LastSeenTime)
		if e != nil {
			return e
		}
	}
	_, e = tx.ExecContext(ctx, `UPDATE panel_routing_state r SET
 healthy_count=(SELECT count(*) FROM panel_relay_assignments a WHERE a.receiver_panel_id=r.panel_id AND a.selected AND a.applied AND a.receiver_plan_hash=r.plan_hash AND a.alive AND a.last_success_at>now()-interval '35 seconds'),
 next_check_at=CASE WHEN EXISTS(SELECT 1 FROM panel_relay_assignments a WHERE a.receiver_panel_id=r.panel_id AND a.selected AND a.cooldown_until>now()) THEN LEAST(r.next_check_at,now()) ELSE r.next_check_at END
 WHERE r.panel_id=$1 AND r.relay_mode`, panel)
	if e != nil {
		return e
	}
	return tx.Commit()
}

// Poll only the local Xray observations, never send duplicate proxy probes.
// Three installed candidate tunnels keep data-plane failover autonomous.
func (s Service) RunRelayHealth(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		supervision.Pulse(ctx)
		rows, e := s.DB.QueryContext(ctx, `SELECT r.panel_id::text FROM panel_routing_state r
   JOIN panel_instances p ON p.id=r.panel_id JOIN droplets dr ON dr.id=p.droplet_id
   WHERE r.relay_mode AND p.enabled AND dr.state='READY' AND (dr.expires_at IS NULL OR dr.expires_at>now()) ORDER BY r.panel_id`)
		ids := []string{}
		if e == nil {
			for rows.Next() {
				var id string
				if rows.Scan(&id) == nil {
					ids = append(ids, id)
				}
			}
			e = rows.Err()
			rows.Close()
		}
		if e != nil && ctx.Err() == nil {
			log.Printf("relay health inventory unavailable")
		}
		// Bounded independent reads; no configuration mutation or lease bypass.
		for start := 0; start < len(ids); start += 4 {
			end := min(start+4, len(ids))
			done := make(chan struct{}, end-start)
			for _, id := range ids[start:end] {
				go func(id string) {
					defer func() { done <- struct{}{} }()
					c, cancel := context.WithTimeout(ctx, 7*time.Second)
					defer cancel()
					if err := supervision.Work(c, func(c context.Context) error { return s.observeRelays(c, id) }); err != nil && ctx.Err() == nil {
						log.Printf("relay health observation unavailable panel=%s", id)
					}
				}(id)
			}
			for i := start; i < end; i++ {
				select {
				case <-done:
				case <-ctx.Done():
					return
				}
			}
		}
		supervision.Idle(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
