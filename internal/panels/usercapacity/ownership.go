package usercapacity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type bulkGeneration struct {
	ID     string
	Marker string
}

func ownershipEmail(marker, clientID string) string {
	short := strings.ReplaceAll(clientID, "-", "")
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("u-%s-%s", marker, short)
}

func ownershipMatches(email, marker string) bool {
	return strings.HasPrefix(email, "u-"+marker+"-")
}

func (s Service) activePolicyGeneration(ctx context.Context, panelID string, inboundID int64) (bulkGeneration, error) {
	var g bulkGeneration
	err := s.DB.QueryRowContext(ctx,
		"SELECT id::text,marker FROM bulk_user_generations WHERE panel_id=$1 AND inbound_id=$2 AND purpose='POLICY' AND state='ACTIVE' ORDER BY created_at DESC LIMIT 1",
		panelID, inboundID,
	).Scan(&g.ID, &g.Marker)
	if err == nil {
		return g, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return g, err
	}

	marker := fmt.Sprintf("p%s-i%d", strings.ReplaceAll(panelID, "-", ""), inboundID)
	err = s.DB.QueryRowContext(ctx,
		"INSERT INTO bulk_user_generations(panel_id,inbound_id,purpose,marker) VALUES($1,$2,'POLICY',$3) ON CONFLICT(marker) DO UPDATE SET marker=excluded.marker RETURNING id::text,marker",
		panelID, inboundID, marker,
	).Scan(&g.ID, &g.Marker)
	return g, err
}

func (s Service) planOwnedClients(ctx context.Context, generationID string, clients []ownedClient) error {
	if len(clients) == 0 {
		return nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, c := range clients {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO bulk_user_ownership(generation_id,client_id,email,state) VALUES($1,$2,$3,'PLANNED') ON CONFLICT(client_id) DO NOTHING",
			generationID, c.ID, c.Email,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type ownedClient struct {
	ID    string
	Email string
}

func (s Service) confirmPlannedOwnedClients(ctx context.Context, panelID string, inboundID int64, observed map[string]string) error {
	rows, err := s.DB.QueryContext(ctx,
		"SELECT o.generation_id::text,o.client_id,o.email,g.marker FROM bulk_user_ownership o JOIN bulk_user_generations g ON g.id=o.generation_id WHERE g.panel_id=$1 AND g.inbound_id=$2 AND o.state='PLANNED'",
		panelID, inboundID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var generationID, clientID, email, marker string
		if err := rows.Scan(&generationID, &clientID, &email, &marker); err != nil {
			return err
		}
		if observed[clientID] != email || !ownershipMatches(email, marker) {
			continue
		}
		if _, err := s.DB.ExecContext(ctx,
			"UPDATE bulk_user_ownership SET state='ACTIVE' WHERE generation_id=$1 AND client_id=$2 AND email=$3 AND state='PLANNED'",
			generationID, clientID, email,
		); err != nil {
			return err
		}
	}
	return rows.Err()
}
