package adminapi

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lib/pq"
)

const residentialBatchLimit = 100

var residentialUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var residentialHost = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`)

type residentialImportRow struct {
	Name string `json:"name"`
	Line string `json:"line"`
}
type residentialImportRequest struct {
	RequestID string                 `json:"request_id"`
	Proxies   []residentialImportRow `json:"proxies"`
}
type residentialIDs struct {
	IDs []string `json:"ids"`
}

func validResidentialName(s string) bool {
	return s != "" && s == strings.TrimSpace(s) && utf8.ValidString(s) && utf8.RuneCountInString(s) <= 80 && !strings.ContainsFunc(s, unicode.IsControl)
}
func parseResidentialLine(line string) (residentialWrite, error) {
	x := residentialWrite{Type: "socks5", Priority: 100, Enabled: true}
	if len(line) > 4096 || strings.ContainsAny(line, "\r\n\x00") {
		return x, errors.New("invalid SOCKS line")
	}
	var rest string
	if strings.HasPrefix(line, "[") {
		end := strings.Index(line, "]:")
		if end < 0 {
			return x, errors.New("use [IPv6]:port:user:password")
		}
		x.Host = line[1:end]
		if net.ParseIP(x.Host) == nil {
			return x, errors.New("invalid IPv6 address")
		}
		rest = line[end+2:]
	} else {
		host, tail, ok := strings.Cut(line, ":")
		if !ok {
			return x, errors.New("expected host:port:user:password")
		}
		x.Host = host
		rest = tail
		if net.ParseIP(host) == nil && (!residentialHost.MatchString(host) || strings.Contains(host, "..")) {
			return x, errors.New("invalid host")
		}
	}
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) != 3 {
		return x, errors.New("expected host:port:user:password")
	}
	p, e := strconv.Atoi(parts[0])
	if e != nil || p < 1 || p > 65535 {
		return x, errors.New("port must be 1 to 65535")
	}
	x.Port = p
	x.Username = parts[1]
	x.Password = parts[2]
	if len(x.Username) > 255 || len(x.Password) > 255 || strings.ContainsFunc(x.Username, unicode.IsControl) || strings.ContainsFunc(x.Password, unicode.IsControl) {
		return x, errors.New("invalid SOCKS credentials")
	}
	if (x.Username == "") != (x.Password == "") {
		return x, errors.New("username and password must both be supplied or both empty")
	}
	return x, nil
}
func decodeResidential(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(dst) != nil || dec.Decode(new(any)) != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "invalid_request"})
		return false
	}
	return true
}
func residentialAdmin(w http.ResponseWriter, r *http.Request) bool {
	p, _ := principal(r.Context())
	if !p.CanAdmin() {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return false
	}
	return true
}
func (s *Server) importResidentialProxies(w http.ResponseWriter, r *http.Request) {
	if !residentialAdmin(w, r) {
		return
	}
	var input residentialImportRequest
	if !decodeResidential(w, r, &input) {
		return
	}
	if !residentialUUID.MatchString(input.RequestID) || len(input.Proxies) < 1 || len(input.Proxies) > residentialBatchLimit {
		writeJSON(w, 400, map[string]string{"error": "invalid_batch", "detail": "Use a request UUID and 1 to 100 rows."})
		return
	}
	entries := make([]residentialWrite, len(input.Proxies))
	names := map[string]bool{}
	for i, row := range input.Proxies {
		x, e := parseResidentialLine(row.Line)
		if e != nil || !validResidentialName(row.Name) {
			detail := "Name must be 1 to 80 characters without surrounding whitespace or control characters."
			if e != nil {
				detail = e.Error()
			}
			writeJSON(w, 422, map[string]any{"error": "invalid_row", "row": i + 1, "detail": fmt.Sprintf("Row %d: %s", i+1, detail)})
			return
		}
		key := strings.ToLower(row.Name)
		if names[key] {
			writeJSON(w, 409, map[string]any{"error": "duplicate_name", "row": i + 1, "detail": fmt.Sprintf("Row %d repeats a name (case-insensitive).", i+1)})
			return
		}
		names[key] = true
		x.Name = row.Name
		entries[i] = x
	}
	raw, _ := json.Marshal(input.Proxies)
	digest := fmt.Sprintf("%x", sha256.Sum256(raw))
	zeroBytes(raw)
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	// Serialize imports by idempotency key before checking the ledger, including response-loss retries.
	if _, e = tx.ExecContext(r.Context(), "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "residential-import:"+strings.ToLower(input.RequestID)); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	var oldHash string
	var oldResponse []byte
	e = tx.QueryRowContext(r.Context(), "SELECT request_hash,response FROM residential_imports WHERE request_id=$1", input.RequestID).Scan(&oldHash, &oldResponse)
	if e == nil {
		if oldHash != digest {
			writeJSON(w, 409, map[string]string{"error": "idempotency_conflict", "detail": "This request ID belongs to a different batch."})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(oldResponse)
		return
	}
	if !errors.Is(e, sql.ErrNoRows) {
		writeJSON(w, 500, errorBody())
		return
	}
	out := []map[string]string{}
	for i, x := range entries {
		var id string
		e = tx.QueryRowContext(r.Context(), `INSERT INTO residential_proxies(proxy_id,name,type,host,port,username,outbound_tag,priority,enabled)
 SELECT id,$1,'socks5',$2,$3,NULLIF($4,''),'residential-ads-'||id::text,100,true FROM (SELECT gen_random_uuid() AS id) t RETURNING proxy_id::text`, x.Name, x.Host, x.Port, x.Username).Scan(&id)
		if e != nil {
			var pe *pq.Error
			if errors.As(e, &pe) && pe.Code == "23505" {
				writeJSON(w, 409, map[string]any{"error": "duplicate_name", "row": i + 1, "detail": fmt.Sprintf("Row %d: name already exists. No rows were saved.", i+1)})
			} else {
				writeJSON(w, 500, errorBody())
			}
			return
		}
		if x.Password != "" {
			if e = s.Container.Secrets.PutResidentialTx(r.Context(), tx, id, "proxy-password", "proxy_password", []byte(x.Password)); e != nil {
				writeJSON(w, 500, errorBody())
				return
			}
			if _, e = tx.ExecContext(r.Context(), "UPDATE residential_proxies SET secret_ref='proxy-password' WHERE proxy_id=$1", id); e != nil {
				writeJSON(w, 500, errorBody())
				return
			}
		}
		out = append(out, map[string]string{"id": id, "name": x.Name})
	}
	response := map[string]any{"created": len(out), "proxies": out}
	saved, _ := json.Marshal(response)
	if _, e = tx.ExecContext(r.Context(), "INSERT INTO residential_imports(request_id,request_hash,response) VALUES($1,$2,$3)", input.RequestID, digest, string(saved)); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if e = tx.Commit(); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	writeJSON(w, 201, response)
}
func decodeResidentialIDs(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	var in residentialIDs
	if !decodeResidential(w, r, &in) {
		return nil, false
	}
	if len(in.IDs) == 0 || len(in.IDs) > 10000 {
		writeJSON(w, 400, map[string]string{"error": "invalid_selection"})
		return nil, false
	}
	seen := map[string]bool{}
	for _, id := range in.IDs {
		key := strings.ToLower(id)
		if !residentialUUID.MatchString(id) || seen[key] {
			writeJSON(w, 400, map[string]string{"error": "invalid_selection"})
			return nil, false
		}
		seen[key] = true
	}
	return in.IDs, true
}
func (s *Server) deleteResidentialProxies(w http.ResponseWriter, r *http.Request) {
	if !residentialAdmin(w, r) {
		return
	}
	ids, ok := decodeResidentialIDs(w, r)
	if !ok {
		return
	}
	// The UI's immutable reviewed ID set is the entire delete scope, never a wildcard.
	// This touches neither account proxies nor their credentials/assignments.
	result, e := s.DB.ExecContext(r.Context(), "DELETE FROM residential_proxies WHERE proxy_id=ANY($1::uuid[])", pq.Array(ids))
	if e != nil {
		writeJSON(w, 409, map[string]string{"error": "delete_pending", "detail": "Refresh and retry the selected IDs."})
		return
	}
	n, _ := result.RowsAffected()
	writeJSON(w, 200, map[string]any{"deleted": n})
}
func (s *Server) exportResidentialProxies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !residentialAdmin(w, r) {
		return
	}
	ids, ok := decodeResidentialIDs(w, r)
	if !ok {
		return
	}
	// Lock endpoint versions across credential reads; edits/deletes cannot produce mismatched exports.
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(r.Context(), `SELECT proxy_id::text,name,type,host,port,COALESCE(username,''),COALESCE(secret_ref,'') FROM residential_proxies WHERE proxy_id=ANY($1::uuid[]) ORDER BY name FOR SHARE`, pq.Array(ids))
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	type entry struct {
		ID, Name, Type, Host, User, Ref string
		Port                            int
	}
	var entries []entry
	for rows.Next() {
		var x entry
		if e = rows.Scan(&x.ID, &x.Name, &x.Type, &x.Host, &x.Port, &x.User, &x.Ref); e != nil {
			break
		}
		entries = append(entries, x)
	}
	re := rows.Err()
	rows.Close()
	if e != nil || re != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if len(entries) != len(ids) {
		writeJSON(w, 409, map[string]string{"error": "selection_changed", "detail": "Refresh the list before copying."})
		return
	}
	// Endpoint SHARE locks serialize API credential edits, which first update the row.
	out := []map[string]string{}
	for _, x := range entries {
		password := ""
		if x.Ref != "" {
			b, e := s.Container.Secrets.GetResidential(r.Context(), x.ID, x.Ref)
			if e != nil {
				writeJSON(w, 409, map[string]string{"error": "credential_unavailable"})
				return
			}
			password = string(b)
			zeroBytes(b)
		}
		host := x.Host
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		line := host + ":" + strconv.Itoa(x.Port) + ":" + x.User + ":" + password
		if x.Type != "socks5" {
			u := url.URL{Scheme: x.Type, Host: net.JoinHostPort(x.Host, strconv.Itoa(x.Port))}
			if x.User != "" || password != "" {
				u.User = url.UserPassword(x.User, password)
			}
			line = u.String()
		}
		out = append(out, map[string]string{"id": x.ID, "name": x.Name, "line": line})
	}
	writeJSON(w, 200, map[string]any{"proxies": out})
}
