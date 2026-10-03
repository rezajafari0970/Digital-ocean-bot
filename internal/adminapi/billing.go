package adminapi

import (
	"context"
	"encoding/json"
)

func (s *Server) accountBilling(ctx context.Context, id string) json.RawMessage {
	var raw []byte
	err := s.DB.QueryRowContext(ctx, `SELECT jsonb_build_object('data',data,'observed_at',observed_at,'last_error',last_error,'fresh',observed_at>now()-interval '10 minutes' AND last_error='') FROM account_billing_snapshots WHERE account_id=$1`, id).Scan(&raw)
	if err != nil {
		return json.RawMessage(`{"data":{},"fresh":false,"last_error":"Billing has not been observed yet"}`)
	}
	return raw
}
