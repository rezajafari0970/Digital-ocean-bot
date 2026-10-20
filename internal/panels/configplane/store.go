package configplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/lib/pq"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"strings"
	"time"
)

type Profile struct {
	ID                string                                `json:"id"`
	Name              string                                `json:"name"`
	Spec              configdriver.Spec                     `json:"spec"`
	Enabled           bool                                  `json:"enabled"`
	Fleet             bool                                  `json:"fleet"`
	Panels            []string                              `json:"panel_ids"`
	Classes           []string                              `json:"classes"`
	Count             int                                   `json:"clients_per_class"`
	ClassSettings     map[string]configdriver.ClassSettings `json:"class_settings,omitempty"`
	Revision          int64                                 `json:"revision"`
	Legacy            string                                `json:"legacy_key,omitempty"`
	RequestID         string                                `json:"request_id,omitempty"`
	DeleteRequestedAt *time.Time                            `json:"delete_requested_at,omitempty"`
	DeletedAt         *time.Time                            `json:"deleted_at,omitempty"`
}
type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func Read(ctx context.Context, db queryer, id string) (Profile, error) {
	p := Profile{}
	var raw, policies []byte
	var legacy sql.NullString
	e := db.QueryRowContext(ctx, "SELECT id::text,name,spec,enabled,fleet,panel_ids,classes,clients_per_class,revision,legacy_key,delete_requested_at,deleted_at,COALESCE(request_id::text,''),class_settings FROM config_driver_profiles WHERE id=$1", id).Scan(&p.ID, &p.Name, &raw, &p.Enabled, &p.Fleet, pq.Array(&p.Panels), pq.Array(&p.Classes), &p.Count, &p.Revision, &legacy, &p.DeleteRequestedAt, &p.DeletedAt, &p.RequestID, &policies)
	if e != nil {
		return p, e
	}
	p.Legacy = legacy.String
	e = json.Unmarshal(raw, &p.Spec)
	if e == nil {
		e = json.Unmarshal(policies, &p.ClassSettings)
	}
	p.Normalize()
	return p, e
}
func (p *Profile) Normalize() {
	if len(p.ClassSettings) == 0 {
		p.ClassSettings = map[string]configdriver.ClassSettings{}
		for _, cl := range p.Classes {
			p.ClassSettings[cl] = configdriver.ClassSettings{Count: p.Count, Flow: p.Spec.Flow, Publish: true}
		}
	}
}
func (p Profile) Total() int {
	p.Normalize()
	n := 0
	for _, cl := range p.Classes {
		n += p.ClassSettings[cl].Count
	}
	return n
}
func (p Profile) PublishedTarget() int {
	p.Normalize()
	n := 0
	for _, cl := range p.Classes {
		if p.ClassSettings[cl].Publish {
			n += p.ClassSettings[cl].Count
		}
	}
	return n
}
func Validate(p Profile) error {
	p.Normalize()
	if strings.TrimSpace(p.Name) == "" || len(p.Name) > 100 || p.Count < 1 || p.Count > 100 || len(p.Classes) < 1 || len(p.Classes) > 2 || len(p.Panels) > 1000 || p.Enabled && !p.Fleet && len(p.Panels) == 0 {
		return errors.New("profile_invalid")
	}
	seen := map[string]bool{}
	for _, c := range p.Classes {
		if (c != "DIRECT" && c != "RESIDENTIAL") || seen[c] {
			return errors.New("profile_classes_invalid")
		}
		seen[c] = true
		settings, ok := p.ClassSettings[c]
		if p.Spec.CloudflareAccount != "" && settings.AllowInsecure {
			return errors.New("profile_cloudflare_strict_tls_required")
		}
		if !ok {
			return errors.New("profile_class_settings_missing")
		}
		if e := settings.Validate(p.Spec); e != nil {
			return e
		}
	}
	for cl := range p.ClassSettings {
		if cl != "DIRECT" && cl != "RESIDENTIAL" {
			return errors.New("profile_classes_invalid")
		}
	}
	// This control plane provisions public cloud nodes. A private-only driver
	// cannot be enabled merely because its protocol exists in the core.
	return configdriver.Builtins().Validate(p.Spec, true)
}

const EligibleTarget = `pi.enabled AND a.enabled AND a.provider_state='ACTIVE' AND a.deletion_requested_at IS NULL
 AND dr.state IN('READY','EXPIRING') AND dep.state='PANEL_COMPLETE'
 AND (dr.expires_at IS NULL OR dr.expires_at>now()+interval '10 seconds')
 AND (cp.fleet OR pi.id=ANY(cp.panel_ids))
 AND NOT EXISTS(SELECT 1 FROM panel_cleanup_targets ct JOIN panel_cleanup_jobs cj ON cj.id=ct.job_id WHERE ct.panel_id=pi.id AND ct.state<>'SUCCEEDED' AND cj.state NOT IN('SUCCEEDED','CANCELLED'))`
const Eligible = EligibleTarget + ` AND cp.enabled AND cp.delete_requested_at IS NULL `

func Event(ctx context.Context, db *sql.DB, p Profile, panel, stage, code string) {
	var target any
	if panel != "" {
		target = panel
	}
	_, _ = db.ExecContext(ctx, "INSERT INTO config_driver_events(profile_id,panel_id,revision,stage,code) VALUES($1,$2,$3,$4,$5)", p.ID, target, p.Revision, stage, code)
}
