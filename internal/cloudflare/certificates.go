package cloudflare

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"reflect"
	"sort"
	"time"
)

//go:embed certificate_node.py
var certificateNode string

// Official cloudflare/cloudflare-docs production/public/ssl/static/origin_ca_rsa_root.pem.
// SHA256 91a8a5567efa6bf941162aa806b3ba476aaddf7867640e53053b35fb225a5dae.
//
//go:embed origin_ca_rsa_root.pem
var originRoot []byte

type Node interface {
	RunInput(context.Context, provisioning.Target, []byte, string, []byte) (provisioning.CommandResult, error)
}

func nodeCall(ctx context.Context, node Node, target provisioning.Target, key []byte, input any) ([]byte, error) {
	data, e := json.Marshal(input)
	if e != nil {
		return nil, e
	}
	command := "python3 -c \"import base64;exec(base64.b64decode('" + base64.StdEncoding.EncodeToString([]byte(certificateNode)) + "'))\""
	result, e := node.RunInput(ctx, target, key, command, data)
	if e != nil {
		return nil, errors.New("cloudflare_certificate_node_retrying")
	}
	return []byte(result.Stdout), nil
}

// EnsureCertificate serializes a single account/node, persists CSR before issue,
// and recovers ambiguous issuance by CSR identity. Private keys never leave the node.
func (s Service) EnsureCertificate(ctx context.Context, account, panel string, node Node, target provisioning.Target, key []byte) (time.Time, error) {
	var expiry time.Time
	e := s.Locked(ctx, "certificate:"+account+":"+panel, func() error {
		d, e := s.Driver(ctx, account)
		if e != nil {
			return e
		}
		var enabled, manage bool
		if e = s.DB.QueryRowContext(ctx, "SELECT enabled,manage_ssl FROM cloudflare_accounts WHERE id=$1", account).Scan(&enabled, &manage); e != nil {
			return e
		}
		if !enabled {
			return errors.New("cloudflare_account_disabled")
		}
		rows, e := s.DB.QueryContext(ctx, "SELECT id,name,COALESCE(checked_at,to_timestamp(0)),ssl_strict FROM cloudflare_zones WHERE account_id=$1 AND enabled AND present AND status='active' ORDER BY name", account)
		if e != nil {
			return e
		}
		type zone struct {
			id, name string
			checked  time.Time
			strict   bool
		}
		zones := []zone{}
		for rows.Next() {
			var z zone
			if e = rows.Scan(&z.id, &z.name, &z.checked, &z.strict); e != nil {
				break
			}
			zones = append(zones, z)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return e
		}
		if len(zones) < 1 || len(zones) > 100 {
			return errors.New("cloudflare_no_active_zones")
		}
		names := []string{}
		for _, z := range zones {
			names = append(names, "*."+z.name)
			if !z.strict || time.Since(z.checked) > 5*time.Minute {
				mode, e := d.Setting(ctx, z.id, "ssl")
				if e != nil {
					return e
				}
				if mode != "strict" && manage {
					if e = d.SetSetting(ctx, z.id, "ssl", "strict"); e != nil {
						return e
					}
					mode, e = d.Setting(ctx, z.id, "ssl")
					if e != nil {
						return e
					}
				}
				if mode != "strict" {
					return errors.New("cloudflare_ssl_strict_required")
				}
				if _, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_zones SET ssl_strict=true,checked_at=now() WHERE account_id=$1 AND id=$2", account, z.id); e != nil {
					return e
				}
			}
		}
		sort.Strings(names)
		var generation, state, csr, cert string
		var oldNames []string
		var until sql.NullTime
		e = s.DB.QueryRowContext(ctx, "SELECT generation::text,names,state,csr,pem,not_after FROM cloudflare_certificates WHERE account_id=$1 AND panel_id=$2", account, panel).Scan(&generation, pq.Array(&oldNames), &state, &csr, &cert, &until)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if e == nil && state == "INSTALLED" && reflect.DeepEqual(names, oldNames) && until.Valid && until.Time.After(time.Now().Add(48*time.Hour)) {
			raw, inspectErr := nodeCall(ctx, node, target, key, map[string]any{"action": "inspect", "account": account, "panel": panel, "generation": generation})
			var out struct {
				SHA string `json:"sha256"`
			}
			sum := sha256.Sum256([]byte(cert))
			if inspectErr == nil && json.Unmarshal(raw, &out) == nil && out.SHA == hex.EncodeToString(sum[:]) {
				expiry = until.Time
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
		// Keep installed certificates and uncertain issuance independent of DNS access.
		// Only new, unsent issuance waits for the domain to become usable.
		if errors.Is(e, sql.ErrNoRows) || state == "PREPARED" {
			if accessErr := s.dnsAccessFailure(ctx, account); accessErr != nil {
				return accessErr
			}
		}
		if errors.Is(e, sql.ErrNoRows) || state == "INSTALLED" {
			generation, e = NewID()
			if e != nil {
				return e
			}
			state = "PREPARED"
			csr = ""
			cert = ""
			_, e = s.DB.ExecContext(ctx, `INSERT INTO cloudflare_certificates(account_id,panel_id,generation,names) VALUES($1,$2,$3,$4) ON CONFLICT(account_id,panel_id) DO UPDATE SET generation=excluded.generation,names=excluded.names,state='PREPARED',csr='',pem='',provider_id='',not_after=NULL,negative_observations=0,last_observed_at=NULL,updated_at=now()`, account, panel, generation, pq.Array(names))
			if e != nil {
				return e
			}
		} else {
			names = oldNames
		} // Finish a journaled generation before changing its desired names.
		input := map[string]any{"action": "prepare", "account": account, "panel": panel, "generation": generation, "names": names}
		if csr == "" {
			raw, e := nodeCall(ctx, node, target, key, input)
			if e != nil {
				return e
			}
			var out struct {
				CSR string `json:"csr"`
			}
			if json.Unmarshal(raw, &out) != nil || out.CSR == "" {
				return errors.New("cloudflare_csr_invalid")
			}
			csr = out.CSR
			if _, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_certificates SET csr=$3,updated_at=now() WHERE account_id=$1 AND panel_id=$2 AND generation=$4", account, panel, csr, generation); e != nil {
				return e
			}
		}
		if state == "SENT" {
			cert = "" // Only a fresh provider observation can resolve an ambiguous issue.
			candidates, e := d.Certificates(ctx, zones[0].id)
			if e != nil {
				return e
			}
			for _, c := range candidates {
				if sameCSR(c.CSR, csr) {
					cert = c.Certificate
					break
				}
			}
			if cert == "" {
				// Two separated authoritative observations plus a five-minute quiet period
				// permit a new CSR/key intent. Never replay an ambiguous issuance request.
				_, _ = s.DB.ExecContext(ctx, "UPDATE cloudflare_certificates SET negative_observations=negative_observations+1,last_observed_at=now() WHERE account_id=$1 AND panel_id=$2 AND generation=$3 AND (last_observed_at IS NULL OR last_observed_at<now()-interval '1 minute')", account, panel, generation)
				var retry bool
				e = s.DB.QueryRowContext(ctx, `SELECT negative_observations>=2 AND updated_at<now()-interval '5 minutes' AND (SELECT count(*) FROM cloudflare_certificate_history h WHERE h.account_id=$1 AND h.panel_id=$2 AND h.state='SENT' AND h.archived_at>now()-interval '1 day')<3 FROM cloudflare_certificates WHERE account_id=$1 AND panel_id=$2 AND generation=$3`, account, panel, generation).Scan(&retry)
				if e != nil {
					return e
				}
				if retry {
					next, e := NewID()
					if e != nil {
						return e
					}
					tx, e := s.DB.BeginTx(ctx, nil)
					if e != nil {
						return e
					}
					defer tx.Rollback()
					if _, e = tx.ExecContext(ctx, `INSERT INTO cloudflare_certificate_history(generation,account_id,panel_id,names,csr,state,provider_id,pem,not_after) SELECT generation,account_id,panel_id,names,csr,state,provider_id,pem,not_after FROM cloudflare_certificates WHERE account_id=$1 AND panel_id=$2 AND generation=$3 ON CONFLICT(generation) DO NOTHING`, account, panel, generation); e != nil {
						return e
					}
					if _, e = tx.ExecContext(ctx, "UPDATE cloudflare_certificates SET generation=$4,state='PREPARED',csr='',pem='',provider_id='',not_after=NULL,negative_observations=0,last_observed_at=NULL,updated_at=now() WHERE account_id=$1 AND panel_id=$2 AND generation=$3", account, panel, generation, next); e != nil {
						return e
					}
					if e = tx.Commit(); e != nil {
						return e
					}
				}
				return errors.New("cloudflare_certificate_issue_unconfirmed")
			}
		}
		if state == "PREPARED" {
			if _, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_certificates SET state='SENT',updated_at=now() WHERE account_id=$1 AND panel_id=$2 AND generation=$3", account, panel, generation); e != nil {
				return e
			}
			issued, e := d.Issue(ctx, csr, names)
			if e != nil {
				var api *APIError
				if errors.As(e, &api) && (api.Status == 400 || api.Status == 401 || api.Status == 403 || api.Status == 429) {
					_, _ = s.DB.ExecContext(ctx, "UPDATE cloudflare_certificates SET state='PREPARED' WHERE account_id=$1 AND panel_id=$2 AND generation=$3", account, panel, generation)
				}
				return e
			}
			cert = issued.Certificate
			if _, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_certificates SET provider_id=$3 WHERE account_id=$1 AND panel_id=$2 AND generation=$4", account, panel, issued.ID, generation); e != nil {
				return e
			}
		}
		roots := s.CertificateRoots
		if len(roots) == 0 {
			roots = originRoot
		}
		expiry, e = VerifyCertificate(cert, csr, names, roots, time.Now())
		if e != nil {
			return e
		}
		if _, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_certificates SET state='ISSUED',pem=$3,not_after=$4,updated_at=now() WHERE account_id=$1 AND panel_id=$2 AND generation=$5", account, panel, cert, expiry, generation); e != nil {
			return e
		}
		input["action"] = "install"
		input["certificate"] = base64.StdEncoding.EncodeToString([]byte(cert))
		raw, e := nodeCall(ctx, node, target, key, input)
		if e != nil {
			return e
		}
		var out struct {
			Installed bool   `json:"installed"`
			SHA       string `json:"sha256"`
		}
		sum := sha256.Sum256([]byte(cert))
		if json.Unmarshal(raw, &out) != nil || !out.Installed || out.SHA != hex.EncodeToString(sum[:]) {
			return errors.New("cloudflare_certificate_install_unconfirmed")
		}
		_, e = s.DB.ExecContext(ctx, "UPDATE cloudflare_certificates SET state='INSTALLED',updated_at=now() WHERE account_id=$1 AND panel_id=$2 AND generation=$3", account, panel, generation)
		return e
	})
	return expiry, e
}
func sameCSR(a, b string) bool {
	pa, _ := pem.Decode([]byte(a))
	pb, _ := pem.Decode([]byte(b))
	return pa != nil && pb != nil && bytes.Equal(pa.Bytes, pb.Bytes)
}
func VerifyCertificate(certPEM, csrPEM string, names []string, root []byte, now time.Time) (time.Time, error) {
	fail := errors.New("cloudflare_certificate_invalid")
	cb, _ := pem.Decode([]byte(certPEM))
	rb, _ := pem.Decode([]byte(csrPEM))
	if cb == nil || rb == nil || len(names) == 0 {
		return time.Time{}, fail
	}
	cert, e := x509.ParseCertificate(cb.Bytes)
	if e != nil {
		return time.Time{}, fail
	}
	csr, e := x509.ParseCertificateRequest(rb.Bytes)
	if e != nil || csr.CheckSignature() != nil {
		return time.Time{}, fail
	}
	ca, e := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if e != nil {
		return time.Time{}, fail
	}
	ra, e := x509.MarshalPKIXPublicKey(csr.PublicKey)
	if e != nil || !bytes.Equal(ca, ra) {
		return time.Time{}, fail
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(root) {
		return time.Time{}, fail
	}
	for _, n := range names {
		if len(n) < 3 || n[:2] != "*." {
			return time.Time{}, fail
		}
		if _, e = cert.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: now, DNSName: "probe." + n[2:]}); e != nil {
			return time.Time{}, fail
		}
	}
	if cert.NotAfter.Before(now.Add(24 * time.Hour)) {
		return time.Time{}, fail
	}
	return cert.NotAfter, nil
}
