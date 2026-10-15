package ipam

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/url"
	"strconv"
	"time"
)

type HealthProof struct {
	EndpointID                                                                                 string
	EndpointRevision                                                                           int64
	NetworkRevision                                                                            string
	PanelID                                                                                    string
	ConfigRevision                                                                             int64
	Port                                                                                       int
	RouteClass                                                                                 string
	ProviderAttached, GuestConfigured, ProtocolOK, RouteIsolationOK, UserNetworkOK, FallbackOK bool
	ObservedAt, ExpiresAt                                                                      time.Time
	EvidenceDigest                                                                             string
}
type Publication struct {
	AccountID, ServerID, PanelID, ClientID, URI, RouteClass string
	ConfigRevision                                          int64
	ClientValidUntil                                        *time.Time
}
type PublishedEndpoint struct{ EndpointID, URI string }

// RecordProof consumes verifier evidence, never a TCP-only or inventory-only success.
// There is deliberately no public HTTP endpoint to manufacture these receipts.
func (s Store) RecordProof(ctx context.Context, p HealthProof) error {
	now := time.Now()
	if p.EndpointRevision < 1 || p.NetworkRevision == "" || p.ConfigRevision < 1 || p.Port < 1 || p.Port > 65535 || (p.RouteClass != "DIRECT" && p.RouteClass != "RESIDENTIAL") || len(p.EvidenceDigest) != 64 || p.ObservedAt.After(now) || now.Sub(p.ObservedAt) > time.Minute || !p.ExpiresAt.After(now) || p.ExpiresAt.After(p.ObservedAt.Add(time.Minute)) || !p.ProviderAttached || !p.GuestConfigured || !p.ProtocolOK || !p.RouteIsolationOK || !p.UserNetworkOK || !p.FallbackOK {
		return ErrEvidence
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var account string
	if e = tx.QueryRowContext(ctx, "SELECT account_id::text FROM ip_endpoints WHERE id=$1", p.EndpointID).Scan(&account); e != nil {
		return e
	}
	if e = LockAccount(ctx, tx, account); e != nil {
		return e
	}
	var allowed bool
	e = tx.QueryRowContext(ctx, `SELECT e.revision=$2 AND e.network_revision=$3 AND e.phase IN('NETWORK_READY','HEALTHY','PUBLISHED')
 AND e.expires_at>clock_timestamp() AND a.state='OWNED'
 AND EXISTS(SELECT 1 FROM ip_network_footprints f WHERE f.endpoint_id=e.id AND f.revision=$3 AND f.state='APPLIED')
 AND NOT EXISTS(SELECT 1 FROM ip_native_delete_fences d WHERE d.account_id=e.account_id AND d.server_id=e.server_id)
 FROM ip_endpoints e JOIN ip_allocations a ON a.id=e.allocation_id WHERE e.id=$1 FOR UPDATE OF e`, p.EndpointID, p.EndpointRevision, p.NetworkRevision).Scan(&allowed)
	if e != nil {
		return e
	}
	if !allowed {
		return ErrProtected
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO ip_endpoint_proofs(endpoint_id,endpoint_revision,network_revision,panel_id,config_revision,port,route_class,
 provider_attached,guest_configured,protocol_ok,route_isolation_ok,user_network_ok,fallback_ok,observed_at,expires_at,evidence_digest)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, p.EndpointID, p.EndpointRevision, p.NetworkRevision, p.PanelID, p.ConfigRevision, p.Port, p.RouteClass, p.ProviderAttached, p.GuestConfigured, p.ProtocolOK, p.RouteIsolationOK, p.UserNetworkOK, p.FallbackOK, p.ObservedAt, p.ExpiresAt, p.EvidenceDigest)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "UPDATE ip_endpoints SET phase=CASE WHEN phase='PUBLISHED' THEN phase ELSE 'HEALTHY' END WHERE id=$1", p.EndpointID)
	if e != nil {
		return e
	}
	return tx.Commit()
}

// Publish commits leases before returning any additional-IP configuration. URI secrets
// are never stored in this ledger: only a digest, client identity and actual validity.
// Callers must additionally enforce their existing account/token/route output predicates.
func (s Store) Publish(ctx context.Context, p Publication) ([]PublishedEndpoint, error) {
	out := []PublishedEndpoint{}
	u, e := url.Parse(p.URI)
	if e != nil || u.Scheme != "vless" || u.User == nil || u.User.Username() != p.ClientID || p.ConfigRevision < 1 || (p.RouteClass != "DIRECT" && p.RouteClass != "RESIDENTIAL") {
		return out, ErrEvidence
	}
	port, e := strconv.Atoi(u.Port())
	if e != nil || port < 1 || port > 65535 {
		return out, ErrEvidence
	}
	if p.ClientValidUntil != nil && !p.ClientValidUntil.After(time.Now()) {
		return out, nil
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	if e = LockAccount(ctx, tx, p.AccountID); e != nil {
		return out, e
	}
	if e = publicationFence(ctx, tx, p.AccountID); e != nil {
		return out, e
	}

	rows, e := tx.QueryContext(ctx, `SELECT e.id::text,host(e.address)
 FROM ip_endpoints e JOIN ip_allocations a ON a.id=e.allocation_id
 JOIN ip_mutation_intents m ON m.id=a.acquisition_operation JOIN ip_approvals g ON g.id=m.approval_id
 JOIN accounts ac ON ac.id=e.account_id
 WHERE e.account_id=$1 AND e.server_id=$2 AND e.phase IN('HEALTHY','PUBLISHED')
 AND EXISTS(SELECT 1 FROM ip_policies pol WHERE pol.account_id=e.account_id AND pol.family=family(e.address) AND pol.policy->>'enabled'='true')
 AND e.expires_at>clock_timestamp() AND a.state='OWNED' AND EXISTS(SELECT 1 FROM ip_network_footprints f WHERE f.endpoint_id=e.id AND f.revision=e.network_revision AND f.state='APPLIED') AND ac.enabled AND ac.deleted_at IS NULL AND ac.deletion_requested_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM ip_native_delete_fences d WHERE d.account_id=e.account_id AND d.server_id=e.server_id)
 AND NOT EXISTS(SELECT 1 FROM ip_mutation_intents op WHERE op.account_id=e.account_id AND op.server_id=e.server_id AND op.state IN('DISPATCHED','UNKNOWN','BLOCKED'))
 AND (($8::timestamptz IS NOT NULL AND g.funded_until>=$8) OR ($8::timestamptz IS NULL AND g.ongoing_funding))
 AND EXISTS(SELECT 1 FROM ip_endpoint_proofs h WHERE h.endpoint_id=e.id AND h.endpoint_revision=e.revision AND h.network_revision=e.network_revision AND
 h.panel_id=$3 AND h.config_revision=$4 AND EXISTS(SELECT 1 FROM residential_routing_control rc JOIN panel_routing_state rs ON rs.revision=rc.revision WHERE rs.panel_id=$3 AND rs.state='APPLIED' AND rs.verified_at>clock_timestamp()-interval '60 seconds' AND rc.revision=$4) AND h.port=$5 AND h.route_class=$6 AND h.expires_at>clock_timestamp() AND h.observed_at>=clock_timestamp()-interval '1 minute'
 AND h.provider_attached AND h.guest_configured AND h.protocol_ok AND h.route_isolation_ok AND h.user_network_ok AND h.fallback_ok)
 AND EXISTS(SELECT 1 FROM panel_instances pi JOIN droplets dr ON dr.id=pi.droplet_id WHERE pi.id=$3 AND pi.account_id=$1 AND dr.account_id=$1 AND dr.provider_resource_id=$2 AND pi.enabled AND dr.state IN('READY','EXPIRING') AND (dr.expires_at IS NULL OR ($8::timestamptz IS NOT NULL AND dr.expires_at>=$8)))
 AND NOT EXISTS(SELECT 1 FROM ip_publication_leases l WHERE l.endpoint_id=e.id AND l.panel_id=$3 AND l.client_id=$7 AND l.migrated_at IS NOT NULL)
 ORDER BY e.id FOR UPDATE OF e`, p.AccountID, p.ServerID, p.PanelID, p.ConfigRevision, port, p.RouteClass, p.ClientID, p.ClientValidUntil)
	if e != nil {
		return out, e
	}
	type target struct{ id, address string }
	targets := []target{}
	for rows.Next() {
		var v target
		if e = rows.Scan(&v.id, &v.address); e != nil {
			rows.Close()
			return out, e
		}
		targets = append(targets, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, v := range targets {
		uri, err := EndpointURI(p.URI, v.address)
		if err != nil {
			return nil, err
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO ip_publication_leases(endpoint_id,panel_id,client_id,uri_digest,route_class,valid_until,port,config_revision)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8)
 ON CONFLICT(endpoint_id,panel_id,client_id,uri_digest) DO UPDATE SET
 last_published_at=clock_timestamp(),
 valid_until=CASE WHEN ip_publication_leases.valid_until IS NULL OR excluded.valid_until IS NULL THEN NULL ELSE GREATEST(ip_publication_leases.valid_until,excluded.valid_until) END
 WHERE ip_publication_leases.migrated_at IS NULL AND ip_publication_leases.route_class=excluded.route_class`, v.id, p.PanelID, p.ClientID, digestBytes([]byte(uri)), p.RouteClass, p.ClientValidUntil, port, p.ConfigRevision)
		if err != nil {
			return nil, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if n != 1 {
			return nil, ErrReceiptConflict
		}
		_, e = tx.ExecContext(ctx, "UPDATE ip_endpoints SET phase='PUBLISHED' WHERE id=$1", v.id)
		if e != nil {
			return nil, e
		}
		out = append(out, PublishedEndpoint{v.id, uri})
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return out, nil
}

// BeginDrain closes publication under the same fence. It does not revoke old
// configurations, mark clients migrated, or free an IP merely because its lifetime ended.
func (s Store) BeginDrain(ctx context.Context, id, replacement string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var account string
	if e = tx.QueryRowContext(ctx, "SELECT account_id::text FROM ip_endpoints WHERE id=$1", id).Scan(&account); e != nil {
		return e
	}
	if e = LockAccount(ctx, tx, account); e != nil {
		return e
	}
	if e = publicationFence(ctx, tx, account); e != nil {
		return e
	}

	var safe bool
	e = tx.QueryRowContext(ctx, `SELECT old.phase IN('HEALTHY','PUBLISHED','DRAINING') AND `+replacementReadySQL+`
 FROM ip_endpoints old JOIN ip_endpoints new ON new.id=$2 WHERE old.id=$1 FOR UPDATE OF old,new`, id, replacement).Scan(&safe)
	if e != nil {
		return e
	}
	if !safe {
		return ErrProtected
	}
	_, e = tx.ExecContext(ctx, "UPDATE ip_endpoints SET phase='DRAINING',replacement_id=$2 WHERE id=$1", id, replacement)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s Store) Retire(ctx context.Context, id string) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var account string
	if e = tx.QueryRowContext(ctx, "SELECT account_id::text FROM ip_endpoints WHERE id=$1", id).Scan(&account); e != nil {
		return e
	}
	if e = LockAccount(ctx, tx, account); e != nil {
		return e
	}
	if e = publicationFence(ctx, tx, account); e != nil {
		return e
	}

	var safe bool
	e = tx.QueryRowContext(ctx, `SELECT old.phase='DRAINING' AND a.state='OWNED' AND `+replacementReadySQL+`
 AND NOT EXISTS(SELECT 1 FROM ip_publication_leases l WHERE l.endpoint_id=old.id AND l.migrated_at IS NULL AND (l.valid_until IS NULL OR l.valid_until>clock_timestamp()))
 AND NOT EXISTS(SELECT 1 FROM ip_mutation_intents m WHERE m.account_id=old.account_id AND m.server_id=old.server_id AND m.state IN('DISPATCHED','UNKNOWN','ACCEPTED','BLOCKED'))
 FROM ip_endpoints old JOIN ip_allocations a ON a.id=old.allocation_id JOIN ip_endpoints new ON new.id=old.replacement_id WHERE old.id=$1 FOR UPDATE OF old,new`, id).Scan(&safe)
	if errors.Is(e, sql.ErrNoRows) {
		return ErrProtected
	}
	if e != nil {
		return e
	}
	if !safe {
		return ErrProtected
	}
	_, e = tx.ExecContext(ctx, "UPDATE ip_endpoints SET phase='RETIRED',retired_at=clock_timestamp(),revision=revision+1 WHERE id=$1", id)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s Store) MarkLost(ctx context.Context, allocation, inventoryRequest string) error {
	// Out-of-band disappearance is never successful migration or release. A full
	// subsequent provider inventory is required and costs/credentials remain retained.
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var account, resource, product string
	var family int
	var observed time.Time
	e = tx.QueryRowContext(ctx, "SELECT account_id::text,provider_resource_id,product,family,last_observed_at FROM ip_allocations WHERE id=$1", allocation).Scan(&account, &resource, &product, &family, &observed)
	if e != nil {
		return e
	}
	if e = LockAccount(ctx, tx, account); e != nil {
		return e
	}

	e = tx.QueryRowContext(ctx, "SELECT provider_resource_id,product,family,last_observed_at FROM ip_allocations WHERE id=$1 AND state<>'RELEASED' FOR UPDATE", allocation).Scan(&resource, &product, &family, &observed)
	if e != nil {
		return e
	}
	var raw []byte
	e = tx.QueryRowContext(ctx, `SELECT i.observation FROM ip_inventory_events i JOIN ip_inventory_lanes l ON l.account_id=i.account_id
 WHERE i.account_id=$1 AND i.request_id=$2 AND i.state='OBSERVED' AND i.started_at>$3
 AND l.state='OBSERVED' AND l.request_id=i.request_id AND l.last_good=i.observation`, account, inventoryRequest, observed).Scan(&raw)
	if e != nil {
		return ErrEvidence
	}
	var o providers.IPObservation
	if e = json.Unmarshal(raw, &o); e != nil {
		return ErrEvidence
	}
	if e = providers.ValidateIPObservation(o); e != nil {
		return e
	}
	var admittedGeneration string
	if e = tx.QueryRowContext(ctx, "SELECT p.body->'input'->>'credential_generation' FROM ip_mutation_intents m JOIN ip_approvals g ON g.id=m.approval_id JOIN ip_plans p ON p.digest=g.plan_digest JOIN ip_allocations a ON a.acquisition_operation=m.id WHERE a.id=$1", allocation).Scan(&admittedGeneration); e != nil {
		return e
	}
	if admittedGeneration != o.CredentialGeneration {
		return ErrEvidence
	}
	if e = currentObservationGeneration(ctx, tx, account, o.CredentialGeneration); e != nil {
		return e
	}
	if o.ObservedAt.After(time.Now()) || !o.ObservedAt.After(observed) || time.Since(o.ObservedAt) > 5*time.Minute {
		return ErrEvidence
	}
	for _, r := range o.Resources {
		if r.ID == resource && r.Kind == product && r.Family == family {
			return ErrEvidence
		}
	}
	if e = retainInventory(ctx, tx, account, inventoryRequest); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO ip_resource_incidents(allocation_id,inventory_request_id,kind) VALUES($1,$2,'OUT_OF_BAND_LOSS') ON CONFLICT DO NOTHING", allocation, inventoryRequest)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "UPDATE ip_allocations SET state='LOST',last_observed_at=$2 WHERE id=$1 AND state<>'RELEASED'", allocation, o.ObservedAt)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "UPDATE ip_endpoints SET phase='LOST',revision=revision+1 WHERE allocation_id=$1 AND retired_at IS NULL", allocation)
	if e != nil {
		return e
	}
	return tx.Commit()
}

const replacementReadySQL = `old.id<>new.id AND old.account_id=new.account_id AND old.server_id=new.server_id
 AND family(old.address)=family(new.address) AND new.phase IN('HEALTHY','PUBLISHED') AND new.expires_at>clock_timestamp()
 AND EXISTS(SELECT 1 FROM ip_allocations a WHERE a.id=new.allocation_id AND a.state='OWNED')
 AND EXISTS(SELECT 1 FROM ip_network_footprints f WHERE f.endpoint_id=new.id AND f.revision=new.network_revision AND f.state='APPLIED')
 AND EXISTS(SELECT 1 FROM ip_endpoint_proofs p JOIN residential_routing_control rc ON rc.revision=p.config_revision
 JOIN panel_routing_state rs ON rs.panel_id=p.panel_id AND rs.revision=rc.revision
 WHERE p.endpoint_id=new.id AND p.endpoint_revision=new.revision AND p.network_revision=new.network_revision AND p.expires_at>clock_timestamp()
 AND p.provider_attached AND p.guest_configured AND p.protocol_ok AND p.route_isolation_ok AND p.user_network_ok AND p.fallback_ok
 AND rs.state='APPLIED' AND rs.verified_at>clock_timestamp()-interval '60 seconds')
 AND NOT EXISTS(
 SELECT 1 FROM (
 SELECT panel_id,port,route_class FROM ip_endpoint_proofs p WHERE p.endpoint_id=old.id AND p.endpoint_revision=old.revision
 UNION SELECT panel_id,port,route_class FROM ip_publication_leases l WHERE l.endpoint_id=old.id AND l.migrated_at IS NULL AND (l.valid_until IS NULL OR l.valid_until>clock_timestamp())
 ) required WHERE NOT EXISTS(
 SELECT 1 FROM ip_endpoint_proofs p JOIN residential_routing_control rc ON rc.revision=p.config_revision
 JOIN panel_routing_state rs ON rs.panel_id=p.panel_id AND rs.revision=rc.revision
 WHERE p.endpoint_id=new.id AND p.endpoint_revision=new.revision AND p.network_revision=new.network_revision AND p.expires_at>clock_timestamp()
 AND p.panel_id=required.panel_id AND p.port=required.port AND p.route_class=required.route_class
 AND p.provider_attached AND p.guest_configured AND p.protocol_ok AND p.route_isolation_ok AND p.user_network_ok AND p.fallback_ok
 AND rs.state='APPLIED' AND rs.verified_at>clock_timestamp()-interval '60 seconds'))`
