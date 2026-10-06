package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"strings"
	"time"
)

type accountBuildState struct {
	Enabled                                             bool
	State, Reason, ProviderFreshness, CapacityFreshness string
	ProviderCanCreate, SchedulerCanBuild                bool
	SchedulerReason                                     string
	Available, Buildable                                *int
	PlanAvailable                                       map[string]int
	Block                                               *capacity.CreateBlock
	DesiredRemaining, Pending, InUse, Limit             int
	LimitKnown                                          bool
}

func (s *Server) accountBuildState(ctx context.Context, id string) (accountBuildState, error) {
	var x accountBuildState
	var runtimeStatus, stateDetail, errorState, errorDetail, provider, accountStatus string
	var observedAt, snapshotAt sql.NullTime
	var desired, managed, preCreate, ops int
	var raw []byte
	err := s.DB.QueryRowContext(ctx, `SELECT a.enabled,a.provider_state,COALESCE(a.provider_state_detail,''),COALESCE(a.provider_error_state,''),COALESCE(a.provider_error_detail,''),a.runtime_status,a.provider_checked_at,a.desired_server_count,
 (SELECT count(*) FROM droplets WHERE account_id=a.id AND state<>'DELETED'),
 (SELECT count(*) FROM deployments WHERE account_id=a.id AND state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE') AND droplet_id IS NULL),
 (SELECT count(*) FROM operations WHERE account_id=a.id AND kind='CREATE_DROPLET' AND COALESCE(resource_id,'')='' AND state IN ('planned','running','verifying','unknown')),
 ps.canonical->'Capacity',ps.created_at,a.provider,COALESCE(ps.canonical->'Account'->>'Status','')
 FROM accounts a LEFT JOIN LATERAL(SELECT canonical,created_at FROM provider_snapshots WHERE account_id=a.id AND canonical IS NOT NULL ORDER BY created_at DESC LIMIT 1) ps ON true WHERE a.id=$1`, id).Scan(&x.Enabled, &x.State, &stateDetail, &errorState, &errorDetail, &runtimeStatus, &observedAt, &desired, &managed, &preCreate, &ops, &raw, &snapshotAt, &provider, &accountStatus)
	if err != nil {
		return x, err
	}
	var c struct {
		LimitKnown                 bool
		ComputeLimit, ComputeInUse int
		PlanAvailable              map[string]int
	}
	if len(raw) > 0 {
		if err = json.Unmarshal(raw, &c); err != nil {
			return x, err
		}
	}
	x.LimitKnown = c.LimitKnown
	x.Limit = c.ComputeLimit
	x.InUse = c.ComputeInUse
	x.PlanAvailable = c.PlanAvailable
	x.Pending = max(preCreate, ops)
	x.DesiredRemaining = max(0, desired-max(managed, c.ComputeInUse)-x.Pending)
	if c.LimitKnown {
		n := max(0, c.ComputeLimit-c.ComputeInUse)
		x.Available = &n
	}
	x.ProviderFreshness = "never"
	x.CapacityFreshness = "never"
	if observedAt.Valid {
		x.ProviderFreshness = observationFreshness(observedAt.Time, time.Now())
	}
	if snapshotAt.Valid {
		x.CapacityFreshness = observationFreshness(snapshotAt.Time, time.Now())
	}
	x.State = strings.ToUpper(x.State)
	x.Block, err = capacity.ReadCreateBlock(ctx, s.DB, id)
	if err != nil {
		return x, err
	}
	x.Reason = "Ready to create servers"
	var meta map[string]any
	_ = json.Unmarshal([]byte(stateDetail), &meta)
	switch {
	case !x.Enabled:
		x.State = "DISABLED"
		x.Reason = "Account disabled in panel"
	case x.Block != nil:
		x.Reason = capacity.CreateBlockReason(x.Block.Code)
	case accountStatus == "trial_restricted":
		x.State = "TRIAL_RESTRICTED"
		x.Reason = capacity.CreateBlockReason("TRIAL_FIREWALL")
	case accountStatus != "" && accountStatus != "active" && x.State == "ACTIVE" && errorState == "":
		x.State = "PROVIDER_WARNING"
		x.Reason = "Provider account status: " + accountStatus
	case x.State != "ACTIVE":
		x.Reason = strings.ReplaceAll(x.State, "_", " ")
		if e, ok := meta["provider_error"].(string); ok && e != "" {
			x.Reason = e
		} else if stateDetail != "" {
			x.Reason = stateDetail
		}
	case errorState != "":
		x.Reason = errorState + ": " + errorDetail
	case x.ProviderFreshness != "fresh":
		x.Reason = "Provider state is " + x.ProviderFreshness
	case x.CapacityFreshness != "fresh":
		x.Reason = "Capacity snapshot is " + x.CapacityFreshness
	case provider == "upcloud" && !c.LimitKnown:
		x.Reason = "Resource budget unverified"
	case c.LimitKnown && *x.Available <= x.Pending:
		x.Reason = "Server capacity is full or reserved by pending builds"
	case runtimeStatus != "READY":
		x.Reason = runtimeStatus
	default:
		x.ProviderCanCreate = true
	}
	x.SchedulerCanBuild = x.ProviderCanCreate && x.DesiredRemaining > 0
	x.SchedulerReason = x.Reason
	if x.ProviderCanCreate {
		if x.DesiredRemaining == 0 {
			x.SchedulerReason = "Desired target reached or reserved"
		} else {
			x.SchedulerReason = "Ready to build"
		}
	}
	// Unknown/stale capacity stays unknown. A definitive block is a known zero.
	knownBlocked := accountStatus == "trial_restricted" || !x.Enabled || x.Block != nil || (x.State != "ACTIVE" && x.State != "UNKNOWN" && x.State != "") || errorState != "" || x.DesiredRemaining == 0
	if knownBlocked {
		n := 0
		x.Buildable = &n
	} else if x.CapacityFreshness == "fresh" && x.ProviderFreshness == "fresh" && c.LimitKnown {
		n := 0
		if x.SchedulerCanBuild {
			n = min(x.DesiredRemaining, max(0, *x.Available-x.Pending))
		}
		x.Buildable = &n
	}
	return x, nil
}
