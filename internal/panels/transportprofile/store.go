package transportprofile

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/lib/pq"
)

type Policy struct {
	Revision int64  `json:"revision"`
	Config   Config `json:"config"`
}

func Read(ctx context.Context, db *sql.DB) (Policy, error) {
	var p Policy
	var raw []byte
	e := db.QueryRowContext(ctx, "SELECT revision,config FROM reality_transport_policy WHERE singleton").Scan(&p.Revision, &raw)
	if e != nil {
		return p, e
	}
	if json.Unmarshal(raw, &p.Config) != nil {
		return p, ErrInvalid
	}
	return p, p.Config.Validate(false)
}

// Resolve runs under the existing panel configuration lock after a fresh
// inventory. Existing TCP listeners remain TCP. A pinned new listener never
// changes its path or transport when the creation default changes.
func Resolve(ctx context.Context, db *sql.DB, panel, key, observed string) (Config, error) {
	var raw []byte
	e := db.QueryRowContext(ctx, "SELECT config FROM reality_listener_profiles WHERE panel_id=$1 AND managed_key=$2", panel, key).Scan(&raw)
	if e == nil {
		var c Config
		if json.Unmarshal(raw, &c) != nil {
			return c, ErrInvalid
		}
		if observed != "" && observed != c.NetworkName() {
			return c, ErrPinned
		}
		return c, c.Validate(c.NetworkName() == "xhttp")
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return Config{}, e
	}
	if observed == "tcp" {
		return Config{}, nil
	}
	if observed != "" {
		return Config{}, ErrPinned
	}
	p, e := Read(ctx, db)
	if e != nil {
		return Config{}, e
	}
	for i := 0; i < 4; i++ {
		c := p.Config
		if c.NetworkName() == "xhttp" {
			c.Path, e = NewPath(c.PathBytes)
			if e != nil {
				return c, e
			}
		}
		raw, e = json.Marshal(c)
		if e != nil {
			return c, e
		}
		var path any
		if c.Path != "" {
			path = c.Path
		}
		_, e = db.ExecContext(ctx, `INSERT INTO reality_listener_profiles(panel_id,managed_key,config,path,policy_revision) VALUES($1,$2,$3,$4,$5) ON CONFLICT(panel_id,managed_key) DO NOTHING`, panel, key, raw, path, p.Revision)
		if e == nil {
			return Resolve(ctx, db, panel, key, observed)
		}
		var pe *pq.Error
		if !errors.As(e, &pe) || pe.Code != "23505" || pe.Constraint != "reality_listener_profiles_path_key" {
			return c, e
		}
	}
	return Config{}, errors.New("could not allocate unique listener path")
}
