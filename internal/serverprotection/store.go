package serverprotection

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"regexp"
	"sort"
	"strings"
)

var ErrConflict = errors.New("protection policy changed; refresh and retry")
var ErrInvalid = errors.New("invalid protection request")
var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Store struct{ DB *sql.DB }
type Request struct {
	RequestID        string   `json:"request_id"`
	ExpectedRevision int64    `json:"expected_revision"`
	Enabled          bool     `json:"enabled"`
	Scope            string   `json:"scope"`
	PanelIDs         []string `json:"panel_ids"`
}
type Control struct {
	Enabled  bool     `json:"enabled"`
	Scope    string   `json:"scope"`
	PanelIDs []string `json:"panel_ids"`
	Revision int64    `json:"revision"`
}

func (s Store) Change(ctx context.Context, q Request) (Control, error) {
	var c Control
	if !uuidRE.MatchString(q.RequestID) || q.ExpectedRevision < 1 || (q.Scope != "fleet" && q.Scope != "selected") || len(q.PanelIDs) > 200 {
		return c, ErrInvalid
	}
	if q.Enabled && q.Scope == "selected" && len(q.PanelIDs) == 0 {
		return c, ErrInvalid
	}
	q.PanelIDs = append([]string(nil), q.PanelIDs...)
	seen := map[string]bool{}
	for i, p := range q.PanelIDs {
		p = strings.ToLower(p)
		if !uuidRE.MatchString(p) || seen[p] {
			return c, ErrInvalid
		}
		seen[p] = true
		q.PanelIDs[i] = p
	}
	sort.Strings(q.PanelIDs)
	if q.PanelIDs == nil {
		q.PanelIDs = []string{}
	}
	raw, _ := json.Marshal(q)
	hash := fmt.Sprintf("%x", sha256.Sum256(raw))
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return c, e
	}
	defer tx.Rollback()
	e = tx.QueryRowContext(ctx, "SELECT enabled,scope,panel_ids,revision FROM server_protection_control WHERE singleton FOR UPDATE").Scan(&c.Enabled, &c.Scope, pq.Array(&c.PanelIDs), &c.Revision)
	if e != nil {
		return c, e
	}
	var oldHash string
	var old []byte
	e = tx.QueryRowContext(ctx, "SELECT request_hash,response FROM server_protection_requests WHERE request_id=$1", q.RequestID).Scan(&oldHash, &old)
	if e == nil {
		if hash != oldHash {
			return c, ErrConflict
		}
		e = json.Unmarshal(old, &c)
		return c, e
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return c, e
	}
	if c.Revision != q.ExpectedRevision {
		return c, ErrConflict
	}
	if q.Enabled && q.Scope == "selected" {
		var count int
		e = tx.QueryRowContext(ctx, `SELECT count(*) FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id JOIN accounts a ON a.id=p.account_id JOIN deployments dp ON dp.droplet_id=d.id
   WHERE p.id=ANY($1) AND p.enabled AND a.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL AND EXISTS(SELECT 1 FROM deployments dp WHERE dp.droplet_id=d.id AND dp.state='PANEL_COMPLETE') AND d.state IN('READY','EXPIRING') AND (d.expires_at IS NULL OR d.expires_at>now()+interval '1 minute') AND dp.state='PANEL_COMPLETE'`, pq.Array(q.PanelIDs)).Scan(&count)
		if e != nil {
			return c, e
		}
		if count != len(q.PanelIDs) {
			return c, ErrInvalid
		}
	}
	c = Control{Enabled: q.Enabled, Scope: q.Scope, PanelIDs: q.PanelIDs, Revision: c.Revision + 1}
	_, e = tx.ExecContext(ctx, "UPDATE server_protection_control SET enabled=$1,scope=$2,panel_ids=$3,revision=$4,updated_at=now() WHERE singleton", c.Enabled, c.Scope, pq.Array(c.PanelIDs), c.Revision)
	if e != nil {
		return c, e
	}
	if e = syncTx(ctx, tx); e != nil {
		return c, e
	}
	raw, _ = json.Marshal(c)
	_, e = tx.ExecContext(ctx, "INSERT INTO server_protection_requests(request_id,request_hash,response) VALUES($1,$2,$3)", q.RequestID, hash, string(raw))
	if e != nil {
		return c, e
	}
	return c, tx.Commit()
}
func (s Store) Sync(ctx context.Context) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var revision int64
	if e = tx.QueryRowContext(ctx, "SELECT revision FROM server_protection_control WHERE singleton FOR UPDATE").Scan(&revision); e != nil {
		return e
	}
	if e = syncTx(ctx, tx); e != nil {
		return e
	}
	return tx.Commit()
}
func syncTx(ctx context.Context, tx *sql.Tx) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO server_protection_nodes(panel_id,desired_revision,desired_enabled,control_revision)
 SELECT p.id,1,true,c.revision FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id JOIN accounts a ON a.id=p.account_id JOIN deployments dp ON dp.droplet_id=d.id CROSS JOIN server_protection_control c
 WHERE c.enabled AND (c.scope='fleet' OR p.id=ANY(c.panel_ids)) AND p.enabled AND a.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL AND dp.state='PANEL_COMPLETE' AND d.state IN('READY','EXPIRING') AND (d.expires_at IS NULL OR d.expires_at>now()+interval '1 minute')
 ON CONFLICT(panel_id) DO NOTHING`)
	if e != nil {
		return e
	}
	// A scope change or disable converges all ever-assigned living nodes, even those
	// excluded from current selection. Eligibility withdrawal is also a disable.
	_, e = tx.ExecContext(ctx, `UPDATE server_protection_nodes n SET desired_revision=n.desired_revision+1,control_revision=c.revision,
 desired_enabled=(c.enabled AND(c.scope='fleet' OR p.id=ANY(c.panel_ids)) AND p.enabled AND a.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL AND EXISTS(SELECT 1 FROM deployments dp WHERE dp.droplet_id=d.id AND dp.state='PANEL_COMPLETE') AND d.state IN('READY','EXPIRING') AND(d.expires_at IS NULL OR d.expires_at>now()+interval '10 seconds')),
 state='PENDING',next_check_at=now(),updated_at=now()
 FROM server_protection_control c,panel_instances p,droplets d,accounts a
 WHERE p.id=n.panel_id AND d.id=p.droplet_id AND a.id=p.account_id AND
 (n.control_revision<>c.revision OR n.desired_enabled IS DISTINCT FROM
 (c.enabled AND(c.scope='fleet' OR p.id=ANY(c.panel_ids)) AND p.enabled AND a.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL AND EXISTS(SELECT 1 FROM deployments dp WHERE dp.droplet_id=d.id AND dp.state='PANEL_COMPLETE') AND d.state IN('READY','EXPIRING') AND(d.expires_at IS NULL OR d.expires_at>now()+interval '10 seconds')))`)
	return e
}
func (s Store) Receipt(ctx context.Context, panel string, p Policy, st Status, receiptErr error) error {
	state := "APPLIED"
	msg := ""
	if !p.Enabled {
		state = "DISABLED"
	}
	if receiptErr == nil {
		receiptErr = st.ValidateReceipt(p)
	}
	if receiptErr != nil {
		state = "ERROR"
		msg = receiptErr.Error()
		if st.State == "UNSUPPORTED" {
			state = "UNSUPPORTED"
		}
		if len(msg) > 400 {
			msg = msg[:400]
		}
	}
	raw, _ := json.Marshal(st)
	res, e := s.DB.ExecContext(ctx, `UPDATE server_protection_nodes SET state=$4,status=$5,last_error=$6,checked_at=now(),next_check_at=now()+interval '15 seconds',updated_at=now(),
 applied_revision=CASE WHEN $6='' THEN $2 ELSE applied_revision END
 WHERE panel_id=$1 AND desired_revision=$2 AND desired_enabled=$3`, panel, p.Revision, p.Enabled, state, string(raw), msg)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	return nil
}
