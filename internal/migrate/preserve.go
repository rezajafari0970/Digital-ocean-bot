package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/lib/pq"
)

type retainedTable struct {
	name     string
	columns  []string
	snapshot string
}
type retentionGuard struct {
	tables    []retainedTable
	schema    string
	sequences map[string]int64
}

// Retention runs in the same transaction as reviewed migration SQL. It permits
// additions, but no loss/change of existing rows or columns. No live data is logged.
func captureRetention(ctx context.Context, tx *sql.Tx, version string) (*retentionGuard, error) {
	g := &retentionGuard{sequences: map[string]int64{}}
	if err := tx.QueryRowContext(ctx, "SELECT current_schema()").Scan(&g.schema); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relkind IN ('r','p') AND NOT c.relispartition ORDER BY c.relname`, g.schema)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// Quiesced installer owns deployment; table locks exclude other database writers.
	for _, name := range names {
		if _, err = tx.ExecContext(ctx, "LOCK TABLE "+pq.QuoteIdentifier(g.schema)+"."+pq.QuoteIdentifier(name)+" IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			return nil, err
		}
	}
	for i, name := range names {
		cols, err := tableColumns(ctx, tx, g.schema, name)
		if err != nil {
			return nil, err
		}
		snapshot := fmt.Sprintf("dob_retained_%d", i)
		projection := "to_jsonb(t)"
		if version == "000172_panel_login_settings" {
			switch name {
			case "admin_users":
				projection += " || jsonb_build_object('must_change_password',false,'bootstrap_token_hash',NULL)"
			case "panel_settings":
				projection += " || jsonb_build_object('listen_host','0.0.0.0')"
			}
		}
		_, err = tx.ExecContext(ctx, "CREATE TEMP TABLE "+pq.QuoteIdentifier(snapshot)+" ON COMMIT DROP AS SELECT "+projection+" AS data FROM "+pq.QuoteIdentifier(g.schema)+"."+pq.QuoteIdentifier(name)+" t")
		if err != nil {
			return nil, err
		}
		g.tables = append(g.tables, retainedTable{name, cols, snapshot})
	}
	rows, err = tx.QueryContext(ctx, `SELECT sequencename,last_value FROM pg_sequences WHERE schemaname=$1 AND last_value IS NOT NULL`, g.schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var value int64
		if err = rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		g.sequences[name] = value
	}
	return g, rows.Err()
}

func tableColumns(ctx context.Context, tx *sql.Tx, schema, name string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT attname FROM pg_attribute WHERE attrelid=to_regclass(quote_ident($1)||'.'||quote_ident($2)) AND attnum>0 AND NOT attisdropped ORDER BY attnum`, schema, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var col string
		if err = rows.Scan(&col); err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	return cols, rows.Err()
}

func (g *retentionGuard) verify(ctx context.Context, tx *sql.Tx) error {
	for _, t := range g.tables {
		current, err := tableColumns(ctx, tx, g.schema, t.name)
		if err != nil {
			return err
		}
		present := map[string]bool{}
		old := map[string]bool{}
		for _, c := range current {
			present[c] = true
		}
		for _, c := range t.columns {
			old[c] = true
			if !present[c] {
				return fmt.Errorf("data retention blocked table/column removal: %s", t.name)
			}
		}
		var added []string
		for _, c := range current {
			if !old[c] {
				added = append(added, c)
			}
		}
		if added == nil {
			added = []string{}
		}
		var missing bool
		query := "SELECT EXISTS(SELECT data FROM " + pq.QuoteIdentifier(t.snapshot) + " EXCEPT ALL SELECT to_jsonb(t)-$1::text[] FROM " + pq.QuoteIdentifier(g.schema) + "." + pq.QuoteIdentifier(t.name) + " t)"
		if err = tx.QueryRowContext(ctx, query, pq.Array(added)).Scan(&missing); err != nil {
			return err
		}
		if missing {
			return fmt.Errorf("data retention blocked deletion or overwrite: %s", t.name)
		}
	}
	for name, prior := range g.sequences {
		var now sql.NullInt64
		err := tx.QueryRowContext(ctx, `SELECT last_value FROM pg_sequences WHERE schemaname=$1 AND sequencename=$2`, g.schema, name).Scan(&now)
		if err != nil || !now.Valid || now.Int64 < prior {
			return fmt.Errorf("data retention blocked sequence reset: %s", name)
		}
	}
	return nil
}

// Recognize top-level transaction commands while skipping quoted SQL bodies,
// identifiers, strings and nested comments. SQL errors remain PostgreSQL's job.
func rejectTransactionEscape(sqlText string) error {
	// setval changes are not rolled back by PostgreSQL. Conservatively block
	// this function even inside migration SQL bodies before executing anything.
	if regexp.MustCompile(`(?i)\bsetval\b`).MatchString(sqlText) {
		return fmt.Errorf("nontransactional sequence mutation is not allowed")
	}
	var words []string
	check := func() error {
		if len(words) == 0 {
			return nil
		}
		switch words[0] {
		case "BEGIN", "COMMIT", "END", "ROLLBACK", "ABORT":
			return fmt.Errorf("migration transaction control is not allowed")
		case "START", "PREPARE":
			if len(words) > 1 && words[1] == "TRANSACTION" {
				return fmt.Errorf("migration transaction control is not allowed")
			}
		}
		return nil
	}
	for i := 0; i < len(sqlText); {
		c := sqlText[i]
		if c == ';' {
			if err := check(); err != nil {
				return err
			}
			words = nil
			i++
			continue
		}
		if i+1 < len(sqlText) && sqlText[i:i+2] == "--" {
			i += 2
			for i < len(sqlText) && sqlText[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < len(sqlText) && sqlText[i:i+2] == "/*" {
			i += 2
			depth := 1
			for i < len(sqlText) && depth > 0 {
				if i+1 < len(sqlText) && sqlText[i:i+2] == "/*" {
					depth++
					i += 2
				} else if i+1 < len(sqlText) && sqlText[i:i+2] == "*/" {
					depth--
					i += 2
				} else {
					i++
				}
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote := c
			escape := quote == '\'' && i > 0 && (sqlText[i-1] == 'E' || sqlText[i-1] == 'e') && (i < 2 || !unicode.IsLetter(rune(sqlText[i-2])))
			i++
			for i < len(sqlText) {
				if sqlText[i] == '\\' && escape {
					i += 2
					continue
				}
				if sqlText[i] == quote {
					if i+1 < len(sqlText) && sqlText[i+1] == quote {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			continue
		}
		if c == '$' {
			end := i + 1
			for end < len(sqlText) && (unicode.IsLetter(rune(sqlText[end])) || unicode.IsDigit(rune(sqlText[end])) || sqlText[end] == '_') {
				end++
			}
			if end < len(sqlText) && sqlText[end] == '$' {
				tag := sqlText[i : end+1]
				rest := strings.Index(sqlText[end+1:], tag)
				if rest < 0 {
					return fmt.Errorf("unterminated migration dollar string")
				}
				i = end + 1 + rest + len(tag)
				continue
			}
		}
		if unicode.IsLetter(rune(c)) || c == '_' {
			end := i + 1
			for end < len(sqlText) && (unicode.IsLetter(rune(sqlText[end])) || unicode.IsDigit(rune(sqlText[end])) || sqlText[end] == '_') {
				end++
			}
			if len(words) < 2 {
				words = append(words, strings.ToUpper(sqlText[i:end]))
			}
			i = end
			continue
		}
		i++
	}
	return check()
}
