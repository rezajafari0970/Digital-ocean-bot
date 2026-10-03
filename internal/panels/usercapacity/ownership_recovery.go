package usercapacity

import (
	"context"
	"errors"
	"time"
)

const (
	plannedRecoveryGrace   = 15 * time.Second
	plannedRecoverySpacing = 5 * time.Second
	plannedRecoveryChecks  = 3
)

type plannedRecoveryAction int

const (
	plannedWait plannedRecoveryAction = iota
	plannedCheck
	plannedAbort
)

func plannedRecoveryDecision(created time.Time, checks int, last *time.Time, now time.Time) plannedRecoveryAction {
	if now.Sub(created) < plannedRecoveryGrace {
		return plannedWait
	}
	if last != nil && now.Sub(*last) < plannedRecoverySpacing {
		return plannedWait
	}
	if checks+1 >= plannedRecoveryChecks {
		return plannedAbort
	}
	return plannedCheck
}

func (s Service) recoverPlannedBarrier(ctx context.Context, panelID string, inboundID int64, observed map[string]string, now time.Time) (bool, error) {
	rows, err := s.DB.QueryContext(ctx, `
SELECT o.generation_id::text,o.client_id,o.email,g.marker,o.created_at,o.recovery_checks,o.last_recovery_check_at
FROM bulk_user_ownership o
JOIN bulk_user_generations g ON g.id=o.generation_id
WHERE g.panel_id=$1 AND g.inbound_id=$2 AND o.state='PLANNED' AND o.mutation_job_id IS NULL
ORDER BY o.created_at,o.client_id
`, panelID, inboundID)
	if err != nil {
		return true, err
	}
	defer rows.Close()
	type item struct {
		generationID, clientID, email, marker string
		created                               time.Time
		checks                                int
		last                                  *time.Time
	}
	var items []item
	for rows.Next() {
		var x item
		if err := rows.Scan(&x.generationID, &x.clientID, &x.email, &x.marker, &x.created, &x.checks, &x.last); err != nil {
			return true, err
		}
		items = append(items, x)
	}
	if err := rows.Err(); err != nil {
		return true, err
	}
	blocked := false
	for _, x := range items {
		if actual, ok := observed[x.clientID]; ok {
			if actual != x.email || !ownershipMatches(actual, x.marker) {
				return true, errors.New("planned ownership marker mismatch")
			}
			if _, err = s.DB.ExecContext(ctx, `UPDATE bulk_user_ownership SET state='ACTIVE',recovery_checks=0,last_recovery_check_at=$4 WHERE generation_id=$1 AND client_id=$2 AND email=$3 AND state='PLANNED'`, x.generationID, x.clientID, x.email, now); err != nil {
				return true, err
			}
			continue
		}
		action := plannedRecoveryDecision(x.created, x.checks, x.last, now)
		if action == plannedWait {
			blocked = true
			continue
		}
		checks := x.checks + 1
		if action == plannedAbort {
			if _, err = s.DB.ExecContext(ctx, `UPDATE bulk_user_ownership SET state='ABORTED',recovery_checks=$3,last_recovery_check_at=$4 WHERE generation_id=$1 AND client_id=$2 AND state='PLANNED'`, x.generationID, x.clientID, checks, now); err != nil {
				return true, err
			}
			continue
		}
		if _, err = s.DB.ExecContext(ctx, `UPDATE bulk_user_ownership SET recovery_checks=$3,last_recovery_check_at=$4 WHERE generation_id=$1 AND client_id=$2 AND state='PLANNED'`, x.generationID, x.clientID, checks, now); err != nil {
			return true, err
		}
		blocked = true
	}
	return blocked, nil
}
