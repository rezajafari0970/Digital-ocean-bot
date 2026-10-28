package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/capacity"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"strings"
	"time"
)

type accountBuildState struct {
	TrialCompatible                                     bool
	TrialProxyCandidates                                int
	TrialRelay                                          *trialRelayStatus
	Enabled                                             bool
	State, Reason, ProviderFreshness, CapacityFreshness string
	ProviderCanCreate, SchedulerCanBuild                bool
	SchedulerReason                                     string
	Available, Buildable                                *int
	PlanAvailable                                       map[string]int
	Block                                               *capacity.CreateBlock
	DesiredRemaining, Pending, InUse, Limit             int
	LimitKnown                                          bool
	RefreshSeconds                                      int
}

func (s *Server) accountBuildState(ctx context.Context, id string) (accountBuildState, error) {
	var x accountBuildState
	var runtimeStatus, stateDetail, errorState, errorDetail, provider, accountStatus, accountMessage string
	var observedAt, snapshotAt sql.NullTime
	var retainedLock bool
	var desired, managed, preCreate, ops, refreshSeconds int
	var raw []byte
	err := s.DB.QueryRowContext(ctx, `SELECT a.enabled,a.provider_state,COALESCE(a.provider_state_detail,''),COALESCE(a.provider_error_state,''),COALESCE(a.provider_error_detail,''),a.runtime_status,a.provider_checked_at,a.desired_server_count,
 (SELECT count(*) FROM droplets WHERE account_id=a.id AND state<>'DELETED'),
 (SELECT count(*) FROM deployments WHERE account_id=a.id AND state NOT IN ('READY','FAILED','INSTALL_FAILED','INSTALL_ROLLED_BACK','PANEL_COMPLETE') AND droplet_id IS NULL),
 (SELECT count(*) FROM operations WHERE account_id=a.id AND kind='CREATE_DROPLET' AND COALESCE(resource_id,'')='' AND state IN ('planned','running','verifying','unknown')),
 ps.canonical->'Capacity',ps.created_at,a.provider,COALESCE(ps.canonical->'Account'->>'Status',''),left(COALESCE(ps.canonical->'Account'->>'StatusMessage',''),513),a.provider_inventory_interval_seconds,a.upcloud_trial_compatible,CASE WHEN a.upcloud_trial_compatible THEN (SELECT count(*) FROM residential_proxies WHERE enabled AND port IN (80,443,8080)) ELSE 0 END,COALESCE(lower(ps.canonical->'Account'->>'Status')='locked' AND (credential.updated_at IS NULL OR credential.updated_at<=ps.created_at) AND (a.provider_account_observed_at IS NULL OR a.provider_account_observed_at<=ps.created_at),false)
 FROM accounts a LEFT JOIN LATERAL(SELECT canonical,created_at FROM provider_snapshots WHERE account_id=a.id AND canonical IS NOT NULL ORDER BY created_at DESC LIMIT 1) ps ON true LEFT JOIN secrets credential ON credential.account_id=a.id AND credential.id=a.secret_ref WHERE a.id=$1`, id).Scan(&x.Enabled, &x.State, &stateDetail, &errorState, &errorDetail, &runtimeStatus, &observedAt, &desired, &managed, &preCreate, &ops, &raw, &snapshotAt, &provider, &accountStatus, &accountMessage, &refreshSeconds, &x.TrialCompatible, &x.TrialProxyCandidates, &retainedLock)
	if err != nil {
		return x, err
	}
	x.RefreshSeconds = refreshSeconds
	if x.TrialCompatible {
		x.TrialRelay, err = s.trialRelayStatus(ctx, id)
		if err != nil {
			return x, err
		}
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
	// "scheduled" explicitly denotes evidence retained until the next planned
	// read. It never passes the strict "fresh" mutation/readiness checks below.
	if observedAt.Valid && x.ProviderFreshness != "fresh" && withinObservationSchedule(observedAt.Time, time.Now(), refreshSeconds) {
		x.ProviderFreshness = "scheduled"
	}
	if snapshotAt.Valid && x.CapacityFreshness != "fresh" && withinObservationSchedule(snapshotAt.Time, time.Now(), refreshSeconds) {
		x.CapacityFreshness = "scheduled"
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
	case x.State == "LOCKED" || x.State == "TOKEN_INVALID" || x.State == "PERMISSION_DENIED" || x.State == "BILLING_BLOCKED":
		x.Reason = strings.ReplaceAll(x.State, "_", " ")
		if e, ok := meta["provider_error"].(string); ok && e != "" {
			x.Reason += ": " + e
		}
	case retainedLock:
		x.State = "LOCKED"
		x.Reason = "Last complete provider account observation is locked; no newer resolution observed. Open the provider control panel and contact support."
		if errorState != "" {
			x.Reason += " Independent observation error: " + errorState
		}
	case x.Block != nil:
		x.Reason = capacity.CreateBlockReason(x.Block.Code)
	case accountStatus == "trial_restricted" && !x.TrialCompatible:
		x.State = "TRIAL_RESTRICTED"
		x.Reason = capacity.CreateBlockReason("TRIAL_FIREWALL")
	case x.State == "ACTIVE" && errorState == "" && (x.CapacityFreshness == "fresh" || x.CapacityFreshness == "scheduled") && providers.AccountCondition(provider, providers.Account{Status: accountStatus, StatusMessage: accountMessage}, providers.Capacity{LimitKnown: c.LimitKnown, ComputeLimit: c.ComputeLimit, ComputeInUse: c.ComputeInUse}) == "CAPACITY_FULL":
		x.State = "CANNOT_CREATE"
		x.Reason = "Provider capacity is full. Existing servers remain usable; new builds wait for verified capacity released by the normal deletion lifecycle."
	case accountStatus != "" && accountStatus != "active" && !(accountStatus == "trial_restricted" && x.TrialCompatible) && x.State == "ACTIVE" && errorState == "":
		x.State = "PROVIDER_WARNING"
		x.Reason = "Provider account status: " + accountStatus
		if accountMessage != "" {
			x.Reason += ". " + accountMessage
		}
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
	// Provider permission is not scheduler admission. A healthy API may coexist
	// with every configured build slot occupied by unresolved provider work.
	blocked, unresolved, expired, err := s.accountSchedulerConcurrency(ctx, id)
	if err != nil {
		return x, err
	}
	if x.SchedulerCanBuild && blocked {
		x.SchedulerCanBuild = false
		x.SchedulerReason = "Concurrent build limit reached; waiting for existing builds"
		if unresolved > 0 {
			x.SchedulerReason = fmt.Sprintf("Concurrent build limit reached: %d create request(s) await a verified provider result. No duplicate create will be sent.", unresolved)
			if expired > 0 {
				x.SchedulerReason += fmt.Sprintf(" %d expired server(s) are excluded from output; retirement waits for pending builds.", expired)
			}
		}
	}
	// Unknown/stale capacity stays unknown. A definitive block is a known zero.
	knownBlocked := blocked || (accountStatus == "trial_restricted" && !x.TrialCompatible) || !x.Enabled || x.Block != nil || (x.State != "ACTIVE" && x.State != "UNKNOWN" && x.State != "") || errorState != "" || x.DesiredRemaining == 0
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

// Use the scheduler's profile-scoped concurrency rule. One saturated schedule
// must not hide another enabled profile that still has room. This is advisory
// display state only; mutation admission always revalidates its own guards.
func (s *Server) accountSchedulerConcurrency(ctx context.Context, id string) (blocked bool, unresolved, expired int, err error) {
	err = s.DB.QueryRowContext(ctx, `WITH configured AS (
  SELECT sc.max_concurrent,(SELECT count(*) FROM deployments d
   WHERE d.account_id=sc.account_id AND d.profile_id=sc.profile_id
   AND d.state IN ('PLANNED','RESERVED','CREATING','WAITING_RESOURCE','PROVISIONING','WAITING_INSTALLER','INSTALL_COMPLETE','IMPORTING_DATABASE','DATABASE_COMPLETE','CONFIGURING_PANEL','REGISTERING_CLIENTS','REGISTERING_TRAFFIC')
   AND (d.droplet_id IS NULL OR EXISTS(SELECT 1 FROM droplets r WHERE r.id=d.droplet_id AND r.state<>'DELETED'))) AS active
  FROM schedules sc JOIN deployment_profiles p ON p.id=sc.profile_id AND p.account_id=sc.account_id
  WHERE sc.account_id=$1 AND sc.enabled AND p.enabled
 ) SELECT EXISTS(SELECT 1 FROM configured) AND NOT EXISTS(SELECT 1 FROM configured WHERE max_concurrent<=0 OR active<max_concurrent),
 (SELECT count(*) FROM operations WHERE account_id=$1 AND kind='CREATE_DROPLET' AND state='unknown' AND COALESCE(resource_id,'')=''),
 (SELECT count(*) FROM droplets WHERE account_id=$1 AND state IN ('READY','EXPIRING') AND expires_at<=now())`, id).Scan(&blocked, &unresolved, &expired)
	return
}
