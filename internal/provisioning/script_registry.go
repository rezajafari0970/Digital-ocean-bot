package provisioning

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

var ErrScriptVersionConflict = errors.New("install script version conflict")
var ErrScriptNotFound = errors.New("install script not found")

type ScriptRef struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}
type ScriptRegistry struct{ DB *sql.DB }

func ScriptHash(s ScriptStep) string {
	payload := struct {
		Name, Category, Precheck, Execute, Verify string
		Timeout                                   int64
		MaxAttempts                               int
	}{s.Name, s.Category, s.Precheck, s.Execute, s.Verify, int64(s.Timeout / time.Second), s.MaxAttempts}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (r ScriptRegistry) Create(ctx context.Context, s ScriptStep) (ScriptStep, error) {
	if r.DB == nil || s.Name == "" || s.Version <= 0 {
		return s, ErrInvalidPlan
	}
	if s.Category == "" {
		s.Category = "script"
	}
	if s.Timeout <= 0 {
		s.Timeout = 10 * time.Minute
	}
	if s.MaxAttempts <= 0 {
		s.MaxAttempts = 3
	}
	s.SHA256 = ScriptHash(s)
	err := r.DB.QueryRowContext(ctx, `INSERT INTO install_scripts(name,version,category,precheck,execute,verify,timeout_seconds,max_attempts,sha256)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT(name,version) DO NOTHING RETURNING id::text`, s.Name, s.Version, s.Category, s.Precheck, s.Execute, s.Verify, int(s.Timeout/time.Second), s.MaxAttempts, s.SHA256).Scan(&s.RegistryID)
	if errors.Is(err, sql.ErrNoRows) {
		var existing string
		if e := r.DB.QueryRowContext(ctx, `SELECT id::text,sha256 FROM install_scripts WHERE name=$1 AND version=$2`, s.Name, s.Version).Scan(&s.RegistryID, &existing); e != nil {
			return s, e
		}
		if existing != s.SHA256 {
			return s, ErrScriptVersionConflict
		}
		return s, nil
	}
	return s, err
}
func (r ScriptRegistry) Resolve(ctx context.Context, refs []ScriptRef) ([]ScriptStep, error) {
	out := make([]ScriptStep, 0, len(refs))
	for _, ref := range refs {
		var s ScriptStep
		var seconds int
		err := r.DB.QueryRowContext(ctx, `SELECT id::text,name,version,category,precheck,execute,verify,timeout_seconds,max_attempts,sha256 FROM install_scripts WHERE name=$1 AND version=$2 AND active=true`, ref.Name, ref.Version).Scan(&s.RegistryID, &s.Name, &s.Version, &s.Category, &s.Precheck, &s.Execute, &s.Verify, &seconds, &s.MaxAttempts, &s.SHA256)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrScriptNotFound
		}
		if err != nil {
			return nil, err
		}
		s.Timeout = time.Duration(seconds) * time.Second
		if ScriptHash(s) != s.SHA256 {
			return nil, ErrScriptVersionConflict
		}
		out = append(out, s)
	}
	return out, nil
}
