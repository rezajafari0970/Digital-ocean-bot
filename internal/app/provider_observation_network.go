package app

import (
	"context"
	"database/sql"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
)

// Hold the precise readiness rows through publication. NOWAIT avoids reversing
// recovery's lock order: contention rolls back and leaves previous evidence intact.
func providerObservationNetworkReady(ctx context.Context, tx *sql.Tx, rt AccountRuntime) (bool, error) {
	var mode, proxy string
	err := tx.QueryRowContext(ctx, `SELECT mode,COALESCE(proxy_id::text,'') FROM network_profiles WHERE account_id=$1 FOR SHARE NOWAIT`, rt.Config.ID).Scan(&mode, &proxy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if mode != string(rt.Config.Network.Mode) {
		return false, nil
	}
	if mode == "direct" {
		return true, nil
	}
	if mode != "proxy_required" || rt.Config.Proxy == nil || proxy != rt.Config.Proxy.ID {
		return false, nil
	}
	var healthy, identityFresh, baseFresh bool
	err = tx.QueryRowContext(ctx, `SELECT status='healthy',COALESCE(last_success_at<=clock_timestamp() AND last_success_at>clock_timestamp()-($2*interval '1 second'),false) FROM proxies WHERE id=$1 FOR SHARE NOWAIT`, proxy, int(network.EconomyBaseMaxAge.Seconds())).Scan(&healthy, &baseFresh)
	if err != nil {
		return false, err
	}
	if !healthy {
		return false, nil
	}
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(last_health_ok AND rotation_started_at IS NULL AND family(exit_ip)=4,false),COALESCE(last_health_at<=clock_timestamp() AND last_health_at>clock_timestamp()-($2*interval '1 second'),false) FROM account_network_identities WHERE account_id=$1 FOR SHARE NOWAIT`, rt.Config.ID, int(network.EconomyIdentityMaxAge.Seconds())).Scan(&healthy, &identityFresh)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !healthy {
		return false, nil
	}
	var generation, epoch int64
	err = tx.QueryRowContext(ctx, `SELECT generation,health_state='healthy' AND circuit_state='closed' FROM proxy_runtime_state WHERE account_id=$1 AND proxy_id=$2 AND provider=$3 FOR SHARE NOWAIT`, rt.Config.ID, proxy, rt.Config.Provider).Scan(&generation, &healthy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !healthy || generation != rt.TransportGeneration {
		return false, nil
	}
	var activeProxy string
	err = tx.QueryRowContext(ctx, `SELECT transport_epoch,COALESCE(active_proxy_id::text,'') FROM account_transport_state WHERE account_id=$1 AND provider=$2 FOR SHARE NOWAIT`, rt.Config.ID, rt.Config.Provider).Scan(&epoch, &activeProxy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if epoch != rt.TransportEpoch || activeProxy != proxy {
		return false, nil
	}
	var economy bool
	err = tx.QueryRowContext(ctx, `SELECT enabled AND (canary_account_id IS NULL OR canary_account_id::text=$1) FROM proxy_economy_policy WHERE singleton FOR SHARE NOWAIT`, rt.Config.ID).Scan(&economy)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	return !economy || (identityFresh && baseFresh), nil
}
