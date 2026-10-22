package app

import (
	"context"
	"database/sql"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/aws"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/ovh"
	"net/http"
	"strings"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/ipam"
)

// nativeIPDeleteTarget recognizes destructive compute requests at the shared
// provider transport boundary, including UpCloud's stop-before-delete sequence.
func nativeIPDeleteTarget(r *http.Request) string {
	if r.URL.Scheme != "https" {
		return ""
	}
	if target := aws.TerminationTarget(r); target != "" {
		return target
	}
	if id, _ := ovh.TerminationTarget(r); id != "" {
		return id
	}
	var prefix, suffix string
	switch r.URL.Host {
	case "api.digitalocean.com":
		if r.Method == http.MethodDelete {
			prefix = "/v2/droplets/"
		}
	case "api.vultr.com":
		if r.Method == http.MethodDelete {
			prefix = "/v2/instances/"
		}
	case "api.linode.com":
		if r.Method == http.MethodDelete {
			prefix = "/v4/linode/instances/"
		}
	case "api.upcloud.com":
		if r.Method == http.MethodDelete {
			prefix = "/1.3/server/"
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/stop") {
			prefix = "/1.3/server/"
			suffix = "/stop"
		}
	}
	if prefix == "" || !strings.HasPrefix(r.URL.Path, prefix) {
		return ""
	}
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), suffix)
	if id == "" || strings.ContainsAny(id, "/%") || len(id) > 128 {
		return ""
	}
	return id
}

// Called while the existing shared account-mutation session fence is held.
// A persistent tombstone closes the paused-before-send race for later IP admission.
// Unknown/rejected native deletion requires explicit reconciliation, never blind removal.
func guardIPNativeDelete(ctx context.Context, conn *sql.Conn, account, server string) error {
	if e := ipam.CheckObligations(ctx, conn, account, server); e != nil {
		return e
	}
	_, e := conn.ExecContext(ctx, `INSERT INTO ip_native_delete_fences(account_id,server_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, account, server)
	return e
}
