package app

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/aws"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ovh"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type providerMutationFence struct {
	base    http.RoundTripper
	db      *sql.DB
	account string
}
type fencedResponseBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (b *fencedResponseBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.release)
	return err
}
func (f providerMutationFence) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodGet || req.Method == http.MethodHead || req.Method == http.MethodOptions {
		return f.base.RoundTrip(req)
	}
	if f.db == nil {
		return nil, errors.New("provider mutation database unavailable")
	}
	conn, err := f.db.Conn(req.Context())
	if err != nil {
		return nil, err
	}
	key := "account-mutation:" + f.account
	if _, err = conn.ExecContext(req.Context(), "SELECT pg_advisory_lock_shared(hashtextextended($1,0))", key); err != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		conn.Close()
		return nil, err
	}
	release := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if _, e := conn.ExecContext(ctx, "SELECT pg_advisory_unlock_shared(hashtextextended($1,0))", key); e != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		conn.Close()
	}
	var deleting, enabled bool
	if err = conn.QueryRowContext(req.Context(), "SELECT deletion_requested_at IS NOT NULL,enabled FROM accounts WHERE id=$1", f.account).Scan(&deleting, &enabled); err != nil {
		release()
		return nil, err
	}
	cleanupStop := false
	if req.Method == http.MethodPost && (deleting || !enabled) && req.URL.Scheme == "https" && req.URL.Host == "api.upcloud.com" && strings.HasPrefix(req.URL.Path, "/1.3/server/") && strings.HasSuffix(req.URL.Path, "/stop") {
		id := strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, "/1.3/server/"), "/stop")
		if id != "" && !strings.Contains(id, "/") {
			if err = conn.QueryRowContext(req.Context(), "SELECT EXISTS(SELECT 1 FROM provider_cleanup_manifests m JOIN accounts a ON a.id=m.account_id WHERE m.account_id=$1 AND a.provider='upcloud' AND m.provider='upcloud' AND m.server_id=$2 AND m.completed_at IS NULL)", f.account, id).Scan(&cleanupStop); err != nil {
				release()
				return nil, err
			}
		}
	}
	if target := aws.TerminationTarget(req); target != "" {
		if err = conn.QueryRowContext(req.Context(), "SELECT EXISTS(SELECT 1 FROM droplets d JOIN accounts a ON a.id=d.account_id WHERE a.id=$1 AND a.provider='aws' AND d.provider_resource_id=$2 AND d.state<>'DELETED')", f.account, target).Scan(&cleanupStop); err != nil || !cleanupStop {
			release()
			return nil, errors.New("AWS termination requires local account resource binding")
		}
	}
	if target := aws.NetworkDeletionTarget(req); target != "" {
		if err = conn.QueryRowContext(req.Context(), `SELECT EXISTS(SELECT 1 FROM provider_account_resources m JOIN accounts a ON a.id=m.account_id WHERE a.id=$1 AND a.provider='aws' AND m.provider='aws' AND NOT a.enabled AND a.deletion_requested_at IS NOT NULL AND m.completed_at IS NULL AND split_part(m.scope,':',1)||':'||m.resource_id=$2) AND NOT EXISTS(SELECT 1 FROM droplets WHERE account_id=$1 AND state<>'DELETED') AND NOT EXISTS(SELECT 1 FROM operations WHERE account_id=$1 AND state NOT IN ('succeeded','failed'))`, f.account, target).Scan(&cleanupStop); err != nil || !cleanupStop {
			release()
			return nil, errors.New("AWS network deletion requires settled account deletion and journal binding")
		}
	}
	if target, external := ovh.TerminationTarget(req); target != "" {
		var bound bool
		if err = conn.QueryRowContext(req.Context(), "SELECT EXISTS(SELECT 1 FROM droplets d JOIN accounts a ON a.id=d.account_id WHERE a.id=$1 AND a.provider='ovh' AND a.external_id=$3 AND d.provider_resource_id=$2 AND d.state<>'DELETED')", f.account, target, external).Scan(&bound); err != nil || !bound {
			release()
			return nil, errors.New("OVH deletion requires local project resource binding")
		}
	}
	if req.Method != http.MethodDelete && !cleanupStop && (deleting || !enabled) {
		release()
		return nil, errors.New("provider mutation blocked: account disabled or deleting")
	}
	if server := nativeIPDeleteTarget(req); server != "" {
		if err = guardIPNativeDelete(req.Context(), conn, f.account, server); err != nil {
			release()
			return nil, err
		}
	}
	resp, err := f.base.RoundTrip(req)
	if err != nil {
		release()
		return nil, err
	}
	if resp.Body == nil {
		release()
	} else {
		resp.Body = &fencedResponseBody{ReadCloser: resp.Body, release: release}
	}
	return resp, nil
}
