package usercapacity

import (
	"context"
	"database/sql"
	"fmt"
)

func (s Service) reconcileShrinkJobs(ctx context.Context, panelID string, inboundID int64) (bool, error) {
	var before int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM bulk_user_ownership o JOIN bulk_user_generations g ON g.id=o.generation_id WHERE g.panel_id=$1 AND g.inbound_id=$2 AND o.state='DELETE_PENDING'`, panelID, inboundID).Scan(&before); err != nil {
		return true, err
	}
	_, err := s.DB.ExecContext(ctx, `
UPDATE bulk_user_ownership o SET state='DELETED',deleted_at=coalesce(o.deleted_at,m.completed_at,now())
FROM bulk_user_generations g, client_mutation_jobs m
WHERE o.generation_id=g.id AND g.panel_id=$1 AND g.inbound_id=$2
AND o.state='DELETE_PENDING' AND m.panel_id=g.panel_id AND m.inbound_id=g.inbound_id
AND m.client_id=o.client_id AND m.kind='DELETE' AND m.idempotency_key='shrink:'||g.panel_id::text||':'||g.inbound_id::text||':'||o.client_id
AND m.state='SUCCEEDED'
`, panelID, inboundID)
	if err != nil {
		return true, err
	}
	var pending int
	err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM bulk_user_ownership o JOIN bulk_user_generations g ON g.id=o.generation_id WHERE g.panel_id=$1 AND g.inbound_id=$2 AND o.state='DELETE_PENDING'`, panelID, inboundID).Scan(&pending)
	return before > 0 || pending > 0, err
}

func (s Service) enqueueShrinkDeletes(ctx context.Context, panelID string, inboundID int64, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var accountID string
	if err = tx.QueryRowContext(ctx, `SELECT account_id::text FROM panel_instances WHERE id=$1 FOR SHARE`, panelID).Scan(&accountID); err != nil {
		return err
	}
	for _, id := range ids {
		key := fmt.Sprintf("shrink:%s:%d:%s", panelID, inboundID, id)
		var jobID string
		err = tx.QueryRowContext(ctx, `
INSERT INTO client_mutation_jobs(account_id,panel_id,inbound_id,client_id,kind,idempotency_key,payload)
VALUES($1,$2,$3,$4,'DELETE',$5,'{}'::jsonb)
ON CONFLICT(account_id,idempotency_key) DO NOTHING RETURNING id::text
`, accountID, panelID, inboundID, id, key).Scan(&jobID)
		if err == sql.ErrNoRows {
			var ep, ec, ek string
			var ei int64
			if err = tx.QueryRowContext(ctx, `SELECT panel_id::text,inbound_id,client_id,kind FROM client_mutation_jobs WHERE account_id=$1 AND idempotency_key=$2`, accountID, key).Scan(&ep, &ei, &ec, &ek); err != nil {
				return err
			}
			if ep != panelID || ei != inboundID || ec != id || ek != "DELETE" {
				return fmt.Errorf("shrink idempotency conflict")
			}
		} else if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE bulk_user_ownership o SET state='DELETE_PENDING' FROM bulk_user_generations g WHERE o.generation_id=g.id AND g.panel_id=$1 AND g.inbound_id=$2 AND o.client_id=$3 AND o.state='ACTIVE'`, panelID, inboundID, id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return fmt.Errorf("shrink ownership transition rejected")
		}
	}
	return tx.Commit()
}
