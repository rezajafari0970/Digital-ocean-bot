package adminapi

import (
	"context"
	"database/sql"
)

func (s *Server) accountFailureHistory(ctx context.Context, id string) ([]accountIssue, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT result,metadata::text,created_at FROM audit_events
 WHERE account_id=$1 AND action='provider.observation.failed'
 ORDER BY created_at DESC,id DESC LIMIT 20`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	history := []accountIssue{}
	for rows.Next() {
		var code, detail string
		var at sql.NullTime
		if err = rows.Scan(&code, &detail, &at); err != nil {
			return nil, err
		}
		history = append(history, accountIssue{"history", code, accountDiagnosticDetail(detail, code), issueTime(at)})
	}
	return history, rows.Err()
}
