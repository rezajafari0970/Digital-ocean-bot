package adminapi

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ovh"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxyselection"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/secrets"
)

var authorizationIDPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

type authorizationSecret struct {
	PreparedToken string `json:"prepared_token,omitempty"`
	Token         string `json:"token"`
	ValidationURL string `json:"validation_url,omitempty"`
}
type authorizationRecord struct {
	ID, Owner, Provider, ProxyID, ProxyCountry, Hash, State, ExternalID, AccountID, LastError string
	ExpiresAt, UpdatedAt                                                                      time.Time
	Cipher                                                                                    secrets.AuthorizationCipher
}

func (s *Server) loadAuthorization(ctx context.Context, owner, id string) (authorizationRecord, error) {
	var x authorizationRecord
	if owner == "" || !authorizationIDPattern.MatchString(id) {
		return x, sql.ErrNoRows
	}
	e := s.DB.QueryRowContext(ctx, `SELECT id::text,owner_id,provider,proxy_id,proxy_country,request_hash,state,external_id,COALESCE(account_id::text,''),last_error,expires_at,updated_at,ciphertext,nonce,COALESCE(key_version,0) FROM provider_authorizations WHERE id=$1 AND owner_id=$2`, id, owner).Scan(&x.ID, &x.Owner, &x.Provider, &x.ProxyID, &x.ProxyCountry, &x.Hash, &x.State, &x.ExternalID, &x.AccountID, &x.LastError, &x.ExpiresAt, &x.UpdatedAt, &x.Cipher.Ciphertext, &x.Cipher.Nonce, &x.Cipher.KeyVersion)
	return x, e
}
func (s *Server) authorizationSecret(x authorizationRecord) (authorizationSecret, error) {
	var out authorizationSecret
	if !x.ExpiresAt.After(time.Now()) || s.Container.Secrets == nil {
		return out, errors.New("authorization expired")
	}
	raw, e := s.Container.Secrets.OpenAuthorization(x.Owner, x.ID, x.Cipher)
	if e != nil {
		return out, e
	}
	defer zeroBytes(raw)
	e = json.Unmarshal(raw, &out)
	return out, e
}
func (s *Server) storeAuthorizationSecret(ctx context.Context, x authorizationRecord, state string, data authorizationSecret, externalID string) error {
	raw, e := json.Marshal(data)
	if e != nil {
		return e
	}
	defer zeroBytes(raw)
	c, e := s.Container.Secrets.SealAuthorization(x.Owner, x.ID, raw)
	if e != nil {
		return e
	}
	result, e := s.DB.ExecContext(ctx, `UPDATE provider_authorizations SET state=$3,ciphertext=$4,nonce=$5,key_version=$6,external_id=$7,last_error='',updated_at=now() WHERE id=$1 AND owner_id=$2 AND expires_at>now() AND state IN ('DISPATCHED','PENDING','READY','VERIFIED')`, x.ID, x.Owner, state, c.Ciphertext, c.Nonce, c.KeyVersion, externalID)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil || n != 1 {
		return errors.New("authorization no longer active")
	}
	return nil
}
func (s *Server) authorizationResponse(w http.ResponseWriter, r *http.Request, id string) {
	p, _ := principal(r.Context())
	x, e := s.loadAuthorization(r.Context(), p.UserID, id)
	if e != nil {
		writeJSON(w, 404, map[string]string{"error": "authorization_not_found"})
		return
	}
	if !x.ExpiresAt.After(time.Now()) && x.State != "USED" {
		x.State = "EXPIRED"
	}
	out := map[string]any{"id": x.ID, "provider": x.Provider, "proxy_id": x.ProxyID, "proxy_country": x.ProxyCountry, "state": x.State, "expires_at": x.ExpiresAt, "retry_after_seconds": 5, "diagnostic": x.LastError}
	if x.LastError != "" && x.Provider == "ovh" {
		out["detail"] = ovh.Diagnostic(&providers.Error{Code: x.LastError}).Guidance
	}
	if x.State == "PENDING" {
		data, e := s.authorizationSecret(x)
		if e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
		out["validation_url"] = data.ValidationURL
	}
	if x.State == "USED" {
		out["account_id"] = x.AccountID
	}
	writeJSON(w, 200, out)
}
func authorizationAdmin(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	p, _ := principal(r.Context())
	if !p.CanAdmin() || p.UserID == "" {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return false
	}
	return true
}
func (s *Server) startProviderAuthorization(w http.ResponseWriter, r *http.Request) {
	if !authorizationAdmin(w, r) {
		return
	}
	if s.Container.Providers == nil {
		writeJSON(w, 400, errorBody())
		return
	}
	provider := r.PathValue("provider")
	a, ok := s.Container.Providers.Authorizer(provider)
	if !ok {
		writeJSON(w, 400, map[string]string{"error": "automatic_authorization_not_supported"})
		return
	}
	var req struct {
		ID           string `json:"id"`
		Token        string `json:"token"`
		ProxyID      string `json:"proxy_id"`
		ProxyCountry string `json:"proxy_country"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || dec.Decode(new(any)) != io.EOF || !authorizationIDPattern.MatchString(req.ID) || req.Token == "" || s.Container.Secrets == nil {
		writeJSON(w, 400, map[string]string{"error": "invalid_authorization_request"})
		return
	}
	var countryErr error
	req.ProxyCountry, countryErr = proxyselection.Country(req.ProxyCountry)
	if countryErr != nil || (req.ProxyCountry != proxyselection.Auto && req.ProxyID == "") {
		writeJSON(w, 400, map[string]string{"error": "invalid_proxy_country_route"})
		return
	}
	unlock, ok := s.authorizationLock(w, r, req.ID)
	if !ok {
		return
	}
	defer unlock()
	p, _ := principal(r.Context())
	hashInput := provider + "\x00" + req.ProxyID + "\x00" + req.Token
	if req.ProxyCountry != proxyselection.Auto {
		hashInput += "\x00country:" + req.ProxyCountry
	}
	sum := sha256.Sum256([]byte(hashInput))
	fingerprint := hex.EncodeToString(sum[:])
	// The durable intent precedes dispatch. Repeating an ID only observes it.
	tx, e := s.DB.BeginTx(r.Context(), nil)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended('provider-authorization:'||$1,0))`, p.UserID); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	var count int
	if e = tx.QueryRowContext(r.Context(), `SELECT count(*) FROM provider_authorizations WHERE owner_id=$1 AND created_at>now()-interval '10 minutes' AND id<>$2`, p.UserID, req.ID).Scan(&count); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	if count >= 6 {
		writeJSON(w, 429, map[string]string{"error": "authorization_rate_limited", "detail": "Too many connection requests. Resume the current request or wait ten minutes."})
		return
	}
	result, e := tx.ExecContext(r.Context(), `INSERT INTO provider_authorizations(id,owner_id,provider,proxy_id,request_hash,state,proxy_country) VALUES($1,$2,$3,$4,$5,'DISPATCHED',$6) ON CONFLICT(id) DO NOTHING`, req.ID, p.UserID, provider, req.ProxyID, fingerprint, req.ProxyCountry)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	n, e := result.RowsAffected()
	if e != nil || tx.Commit() != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	x, e := s.loadAuthorization(r.Context(), p.UserID, req.ID)
	if e != nil || x.Hash != fingerprint {
		writeJSON(w, 409, map[string]string{"error": "authorization_request_conflict"})
		return
	}
	if n == 0 {
		s.authorizationResponse(w, r, req.ID)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 25*time.Second)
	defer cancel()
	rr := r.WithContext(ctx)
	client, closeFn, _, ok := s.previewClient(w, rr, req.ProxyID, req.ProxyCountry, false)
	if !ok {
		_, _ = s.DB.ExecContext(ctx, `UPDATE provider_authorizations SET state='FAILED',last_error='CONNECTION_NOT_READY',updated_at=now() WHERE id=$1 AND state='DISPATCHED'`, req.ID)
		return
	}
	defer closeFn()
	raw := []byte(req.Token)
	defer zeroBytes(raw)
	grant, e := a.StartAuthorization(ctx, providers.OpenRequest{AccountID: "authorization", HTTPClient: client, Credentials: previewCredential{raw}})
	closeFn()
	saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer saveCancel()
	if e != nil {
		state := "FAILED"
		if providers.Class(e) == providers.ErrorAmbiguousOutcome || providers.Class(e) == providers.ErrorTransport || providers.Class(e) == providers.ErrorUnavailable {
			state = "UNKNOWN"
		}
		diagnostic := ovh.Diagnostic(e)
		code := diagnostic.Code
		var providerError *providers.Error
		httpStatus := 0
		if errors.As(e, &providerError) {
			httpStatus = providerError.StatusCode
		}
		log.Printf("provider authorization result ref=%s state=%s code=%s endpoint=%s operation=%s http=%d", req.ID, state, code, diagnostic.Endpoint, diagnostic.Operation, httpStatus)
		_, _ = s.DB.ExecContext(saveCtx, `UPDATE provider_authorizations SET state=$2,last_error=$3,updated_at=now() WHERE id=$1 AND state='DISPATCHED'`, req.ID, state, code)
		s.authorizationResponse(w, rr.WithContext(saveCtx), req.ID)
		return
	}
	raw, e = grant.Credentials.Get(saveCtx)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	defer zeroBytes(raw)
	if e = s.storeAuthorizationSecret(saveCtx, x, "PENDING", authorizationSecret{Token: string(raw), ValidationURL: grant.ValidationURL}, ""); e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	s.authorizationResponse(w, rr.WithContext(saveCtx), req.ID)
}
func (s *Server) providerAuthorizationStatus(w http.ResponseWriter, r *http.Request) {
	if !authorizationAdmin(w, r) {
		return
	}
	p, _ := principal(r.Context())
	id := r.PathValue("id")
	unlock, ok := s.authorizationLock(w, r, id)
	if !ok {
		return
	}
	defer unlock()
	x, e := s.loadAuthorization(r.Context(), p.UserID, id)
	if e != nil {
		writeJSON(w, 404, map[string]string{"error": "authorization_not_found"})
		return
	}
	if x.State != "PENDING" || !x.ExpiresAt.After(time.Now()) {
		s.authorizationResponse(w, r, id)
		return
	}
	// At most one bounded read per flow every ten seconds, across processes.
	result, e := s.DB.ExecContext(r.Context(), `UPDATE provider_authorizations SET next_check_at=now()+interval '10 seconds' WHERE id=$1 AND state='PENDING' AND expires_at>now() AND next_check_at<=now()`, id)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		s.authorizationResponse(w, r, id)
		return
	}
	data, e := s.authorizationSecret(x)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	client, closeFn, _, ok := s.previewClient(w, r, x.ProxyID, x.ProxyCountry, false)
	if !ok {
		return
	}
	defer closeFn()
	a, ok := s.Container.Providers.Authorizer(x.Provider)
	if !ok {
		writeJSON(w, 400, errorBody())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	ready, e := a.CheckAuthorization(ctx, providers.OpenRequest{AccountID: "authorization", HTTPClient: client, Credentials: previewCredential{[]byte(data.Token)}})
	closeFn()
	if e != nil {
		code := ovh.Diagnostic(e).Code
		state := "PENDING"
		if providers.Class(e) == providers.ErrorPermissionDenied || providers.Class(e) == providers.ErrorAuthentication {
			state = "FAILED"
		}
		_, _ = s.DB.ExecContext(r.Context(), `UPDATE provider_authorizations SET state=$2,last_error=$3,updated_at=now(),ciphertext=CASE WHEN $2='FAILED' THEN NULL ELSE ciphertext END,nonce=CASE WHEN $2='FAILED' THEN NULL ELSE nonce END,key_version=CASE WHEN $2='FAILED' THEN NULL ELSE key_version END WHERE id=$1 AND state='PENDING'`, id, state, code)
	} else if ready {
		_, e = s.DB.ExecContext(r.Context(), `UPDATE provider_authorizations SET state='READY',last_error='',updated_at=now() WHERE id=$1 AND state='PENDING' AND expires_at>now()`, id)
		if e != nil {
			writeJSON(w, 500, errorBody())
			return
		}
	}
	s.authorizationResponse(w, r, id)
}
func (s *Server) cancelProviderAuthorization(w http.ResponseWriter, r *http.Request) {
	if !authorizationAdmin(w, r) {
		return
	}
	p, _ := principal(r.Context())
	id := r.PathValue("id")
	if !authorizationIDPattern.MatchString(id) {
		writeJSON(w, 404, errorBody())
		return
	}
	unlock, ok := s.authorizationLock(w, r, id)
	if !ok {
		return
	}
	defer unlock()
	_, e := s.DB.ExecContext(r.Context(), `UPDATE provider_authorizations SET state='CANCELLED',ciphertext=NULL,nonce=NULL,key_version=NULL,updated_at=now() WHERE id=$1 AND owner_id=$2 AND state<>'USED'`, id, p.UserID)
	if e != nil {
		writeJSON(w, 500, errorBody())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) authorizationCredential(ctx context.Context, id, provider, proxyID string, verified bool, countries ...string) (authorizationRecord, authorizationSecret, error) {
	country := proxyselection.Auto
	if len(countries) > 0 {
		var err error
		country, err = proxyselection.Country(countries[0])
		if err != nil {
			return authorizationRecord{}, authorizationSecret{}, err
		}
	}
	p, _ := principal(ctx)
	x, e := s.loadAuthorization(ctx, p.UserID, id)
	if e != nil || x.Provider != provider || x.ProxyID != proxyID || x.ProxyCountry != country || !x.ExpiresAt.After(time.Now()) ||
		(verified && x.State != "VERIFIED") || (!verified && x.State != "READY" && x.State != "VERIFIED") {
		return x, authorizationSecret{}, errors.New("authorization not ready for this account and connection")
	}
	data, e := s.authorizationSecret(x)
	if verified {
		data.Token = data.PreparedToken
		if data.Token == "" {
			e = errors.New("authorization preview required")
		}
	}
	return x, data, e
}

// Cleanup is local only: it never revokes an account credential or retries a
// credential request. Tombstones preserve idempotency after secret expiry.

func (s *Server) tryAuthorizationLock(ctx context.Context, id string) (func(), bool, error) {
	conn, err := s.DB.Conn(ctx)
	if err != nil {
		return nil, false, err
	}
	var locked bool
	err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended('provider-authorization-save:'||$1,0))`, id).Scan(&locked)
	if err != nil || !locked {
		conn.Close()
		return nil, false, err
	}
	return func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := conn.ExecContext(c, `SELECT pg_advisory_unlock(hashtextextended('provider-authorization-save:'||$1,0))`, id)
		if err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		conn.Close()
	}, true, nil
}
func (s *Server) authorizationLock(w http.ResponseWriter, r *http.Request, id string) (func(), bool) {
	if !authorizationIDPattern.MatchString(id) {
		writeJSON(w, 400, map[string]string{"error": "invalid_authorization_request"})
		return nil, false
	}
	unlock, ok, err := s.tryAuthorizationLock(r.Context(), id)
	if err != nil {
		writeJSON(w, 500, errorBody())
		return nil, false
	}
	if !ok {
		w.Header().Set("Retry-After", "2")
		writeJSON(w, 409, map[string]string{"error": "authorization_in_progress"})
		return nil, false
	}
	return unlock, true
}
func (s *Server) CleanupProviderAuthorizations(ctx context.Context) error {
	rows, e := s.DB.QueryContext(ctx, `SELECT id::text FROM provider_authorizations WHERE (expires_at<=now() AND state NOT IN ('USED','EXPIRED','CANCELLED')) OR (state='DISPATCHED' AND updated_at<now()-interval '45 seconds') ORDER BY updated_at LIMIT 100`)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		unlock, ok, err := s.tryAuthorizationLock(ctx, id)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		_, err = s.DB.ExecContext(ctx, `UPDATE provider_authorizations SET state=CASE WHEN expires_at<=now() THEN 'EXPIRED' ELSE 'UNKNOWN' END,last_error=CASE WHEN expires_at<=now() THEN '' ELSE 'AUTHORIZATION_OUTCOME_UNKNOWN' END,ciphertext=NULL,nonce=NULL,key_version=NULL,updated_at=now() WHERE id=$1 AND ((expires_at<=now() AND state NOT IN ('USED','EXPIRED','CANCELLED')) OR (state='DISPATCHED' AND updated_at<now()-interval '45 seconds'))`, id)
		unlock()
		if err != nil {
			return err
		}
	}
	return nil
}
func (s *Server) RunProviderAuthorizationCleanup(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := s.CleanupProviderAuthorizations(c); err != nil && ctx.Err() == nil {
			log.Printf("provider authorization cleanup failed")
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
