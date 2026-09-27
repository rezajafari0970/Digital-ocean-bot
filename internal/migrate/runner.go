package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Runner struct {
	DB  *sql.DB
	Dir string
}

func (r Runner) Up(ctx context.Context) error {
	conn, err := r.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock(728391447221)`); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(728391447221)`)
	if r.Dir == "" {
		r.Dir = "migrations"
	}
	if _, err = conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version TEXT PRIMARY KEY,applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `ALTER TABLE schema_migrations ADD COLUMN IF NOT EXISTS checksum TEXT`); err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(r.Dir, "*.up.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, path := range files {
		version := strings.TrimSuffix(filepath.Base(path), ".up.sql")
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		checksum := hex.EncodeToString(sum[:])
		var stored sql.NullString
		err = conn.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version=$1`, version).Scan(&stored)
		if err == nil {
			if stored.Valid && stored.String != "" && stored.String != checksum {
				return fmt.Errorf("migration %s checksum mismatch", version)
			}
			if !stored.Valid || stored.String == "" {
				if _, e := conn.ExecContext(ctx, `UPDATE schema_migrations SET checksum=$2 WHERE version=$1 AND checksum IS NULL`, version, checksum); e != nil {
					return e
				}
			}
			continue
		}
		if err != sql.ErrNoRows {
			return err
		}
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(raw)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", version, err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,checksum) VALUES($1,$2)`, version, checksum); err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
