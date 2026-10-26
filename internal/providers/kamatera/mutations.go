package kamatera

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers/cloudutil"
	"strconv"
	"strings"
)

func (d *Driver) intentName(identity string) string {
	b := sha256.Sum256([]byte("kamatera-v1\x00" + d.accountID + "\x00" + identity))
	return "dob-" + hex.EncodeToString(b[:16])
}
func validIdentity(identity string) bool {
	return strings.HasPrefix(identity, "dob-deployment-") && uuidPattern.MatchString(strings.TrimPrefix(identity, "dob-deployment-"))
}
func pendingCreate() error {
	return problem(providers.ErrorAmbiguousOutcome, "create_server", "CREATE_AWAITING_VERIFIED_SERVER_ID", 0)
}
func (d *Driver) matchIntent(ctx context.Context, s providers.AccountResourceSession) ([]providers.Server, error) {
	r := s.Record()
	if r.Complete {
		return nil, problem(providers.ErrorInvalidRequest, "create_server", "INTENT_ALREADY_COMPLETED", 0)
	}
	xs, e := d.ListServers(ctx)
	if e != nil {
		return nil, e
	}
	matches := []providers.Server{}
	for _, x := range xs {
		if x.Name == r.Name {
			matches = append(matches, x)
		}
	}
	if len(matches) > 1 {
		return nil, problem(providers.ErrorUnavailable, "create_server", "AMBIGUOUS_SERVER_IDENTITY", 200)
	}
	if len(matches) == 1 {
		var intent computeIntent
		if json.Unmarshal([]byte(r.Intent), &intent) != nil || intent.Region == "" || intent.Region != matches[0].RegionID {
			return nil, problem(providers.ErrorUnavailable, "create_server", "SERVER_INTENT_MISMATCH", 0)
		}
		if !r.Attempted || (r.ID != "" && r.ID != matches[0].ID) {
			return nil, problem(providers.ErrorPermissionDenied, "create_server", "SERVER_OWNERSHIP_CONFLICT", 0)
		}
		if e = s.Bind(ctx, matches[0].ID); e != nil {
			return nil, e
		}
		matches[0].Tags = []string{"managed-by-digital-ocean-bot", "dob-account-" + d.accountID, strings.TrimPrefix(r.Scope, "server:")}
	}
	return matches, nil
}
func (d *Driver) FindServerByIdentity(ctx context.Context, identity string) ([]providers.Server, error) {
	if !validIdentity(identity) || d.resources == nil {
		return nil, problem(providers.ErrorUnavailable, "find_server", "DURABLE_INTENT_REQUIRED", 0)
	}
	var out []providers.Server
	e := d.resources.WithLock(ctx, "server:"+identity, d.intentName(identity), func(s providers.AccountResourceSession) error {
		r := s.Record()
		if !r.Attempted {
			return nil
		}
		var e error
		out, e = d.matchIntent(ctx, s)
		if e != nil {
			return e
		}
		if len(out) == 0 && r.Attempted {
			return pendingCreate()
		}
		return nil
	})
	return out, e
}
func (d *Driver) CreateServer(ctx context.Context, r providers.CreateServerRequest) (providers.CreateServerResult, error) {
	rejected := providers.CreateServerResult{ComputeNotDispatched: true, Outcome: providers.OutcomeRejected}
	if !validIdentity(r.Identity) || !regionPattern.MatchString(r.RegionID) || d.resources == nil || len(r.SSHAuthorizedKeys) != 1 {
		return rejected, problem(providers.ErrorInvalidRequest, "create_server", "INVALID_CREATE_INTENT", 0)
	}
	if _, e := cloudutil.CloudInit(r.SSHAuthorizedKeys); e != nil {
		return rejected, problem(providers.ErrorInvalidRequest, "create_server", "INVALID_SSH_KEY", 0)
	}
	sizes, e := d.sizes(ctx, r.RegionID)
	if e != nil {
		return rejected, e
	}
	images, e := d.images(ctx, r.RegionID)
	if e != nil {
		return rejected, e
	}
	var size *sizeData
	var image *imageData
	for i := range sizes {
		if sizes[i].valid() && sizes[i].planID() == r.PlanID {
			size = &sizes[i]
			break
		}
	}
	if size == nil {
		suggested, e := d.suggestedSizes(ctx, r.RegionID)
		if e != nil {
			return rejected, e
		}
		for i := range suggested {
			if suggested[i].planID() == r.PlanID {
				size = &suggested[i]
				break
			}
		}
	}
	for i := range images {
		if images[i].ID == r.ImageID && images[i].version() != "" {
			image = &images[i]
			break
		}
	}
	if size == nil || image == nil || size.Disk < image.Disk || image.RAM[size.CPUType] <= 0 || size.RAM < image.RAM[size.CPUType] {
		return rejected, problem(providers.ErrorInvalidRequest, "create_server", "INCOMPATIBLE_REGIONAL_PLAN_IMAGE", 0)
	}
	result := rejected
	e = d.resources.WithLock(ctx, "server:"+r.Identity, d.intentName(r.Identity), func(s providers.AccountResourceSession) error {
		matches, e := d.matchIntent(ctx, s)
		if e != nil {
			return e
		}
		if len(matches) == 1 {
			result = providers.CreateServerResult{ServerID: matches[0].ID, Outcome: providers.OutcomeAccepted}
			return nil
		}
		recorder, ok := s.(providers.AsyncResourceSession)
		if !ok {
			return problem(providers.ErrorUnavailable, "create_server", "DURABLE_INTENT_REQUIRED", 0)
		}
		intent, _ := json.Marshal(computeIntent{Region: r.RegionID, Plan: r.PlanID, Image: r.ImageID})
		if e = recorder.PrepareIntent(ctx, string(intent)); e != nil {
			return e
		}
		if s.Record().Attempted {
			result = providers.CreateServerResult{Outcome: providers.OutcomeAmbiguous}
			return pendingCreate()
		}
		started, e := s.BeginAttempt(ctx)
		if e != nil {
			return e
		}
		if !started {
			return pendingCreate()
		}
		// Persist intent before dispatch. A lost response is observation-only forever;
		// an empty inventory never authorizes a second billable POST.
		result = providers.CreateServerResult{Outcome: providers.OutcomeAmbiguous}
		body := map[string]any{"name": s.Record().Name, "password": "", "passwordValidate": "", "ssh-key": strings.TrimSpace(r.SSHAuthorizedKeys[0]), "datacenter": r.RegionID, "image": r.ImageID, "cpu": strconv.Itoa(size.Cores) + size.CPUType, "ram": size.RAM, "disk": "size=" + strconv.Itoa(size.Disk), "dailybackup": "no", "managed": "no", "network": "name=wan,ip=auto", "quantity": 1, "billingcycle": "hourly", "monthlypackage": size.Traffic, "poweronaftercreate": "yes"}
		var ids []json.Number
		if e = d.client.do(ctx, "POST", "/service/server", body, &ids); e != nil {
			// An explicit rejection can close this one intent. Unknown outcomes retain it.
			if !cloudutil.Ambiguous(e) {
				if e2 := s.Finish(ctx); e2 != nil {
					return e2
				}
				result = providers.CreateServerResult{Outcome: providers.OutcomeRejected}
			}
			return e
		}
		if len(ids) != 1 {
			return pendingCreate()
		}
		n, e := strconv.ParseUint(string(ids[0]), 10, 64)
		if e != nil || n == 0 {
			return pendingCreate()
		}
		result.OperationID = string(ids[0])
		if e = recorder.RecordOperation(ctx, result.OperationID); e != nil {
			return e
		}
		return pendingCreate()
	})
	return result, e
}
func (d *Driver) DeleteServer(ctx context.Context, id string) error {
	if !uuidPattern.MatchString(id) || d.resources == nil {
		return problem(providers.ErrorInvalidRequest, "delete_server", "DURABLE_SERVER_BINDING_REQUIRED", 0)
	}
	records, e := d.resources.List(ctx)
	if e != nil {
		return e
	}
	var record *providers.AccountResource
	for i := range records {
		if records[i].ID == id && strings.HasPrefix(records[i].Scope, "server:") {
			if record != nil {
				return problem(providers.ErrorUnavailable, "delete_server", "AMBIGUOUS_SERVER_BINDING", 0)
			}
			record = &records[i]
		}
	}
	if record == nil {
		return problem(providers.ErrorPermissionDenied, "delete_server", "SERVER_NOT_OWNED", 0)
	}
	return d.resources.WithLock(ctx, record.Scope, record.Name, func(s providers.AccountResourceSession) error {
		x, e := d.GetServer(ctx, id)
		if providers.IsClass(e, providers.ErrorNotFound) {
			return s.Finish(ctx)
		}
		if e != nil {
			return e
		}
		if x.Name != s.Record().Name || s.Record().ID != id {
			return problem(providers.ErrorPermissionDenied, "delete_server", "SERVER_OWNERSHIP_CONFLICT", 0)
		}
		recorder, ok := s.(providers.AsyncResourceSession)
		if !ok {
			return problem(providers.ErrorUnavailable, "delete_server", "DURABLE_INTENT_REQUIRED", 0)
		}
		// Retry only the same freshly observed, durably bound UUID, at most
		// once per two minutes. Acceptance still never means absence.
		started, e := recorder.BeginDeletion(ctx)
		if e != nil {
			return e
		}
		if !started {
			return nil
		}
		var ids []json.Number
		if e = d.client.do(ctx, "POST", "/service/server/terminate", map[string]any{"id": id, "force": true}, &ids); e != nil {
			return e
		}
		if len(ids) != 1 {
			return problem(providers.ErrorAmbiguousOutcome, "delete_server", "DELETE_OUTCOME_UNKNOWN", 0)
		}
		// An accepted asynchronous termination is not absence; lifecycle reads confirm it.
		return nil
	})
}
func (d *Driver) CleanupAccountResources(ctx context.Context) error {
	if d.resources == nil {
		return problem(providers.ErrorUnavailable, "cleanup", "DURABLE_INTENT_REQUIRED", 0)
	}
	records, e := d.resources.List(ctx)
	if e != nil {
		return e
	}
	for _, r := range records {
		e = d.resources.WithLock(ctx, r.Scope, r.Name, func(s providers.AccountResourceSession) error {
			x := s.Record()
			if !x.Attempted {
				return s.Finish(ctx)
			}
			if x.ID == "" {
				return pendingCreate()
			}
			_, e := d.GetServer(ctx, x.ID)
			if providers.IsClass(e, providers.ErrorNotFound) {
				return s.Finish(ctx)
			}
			if e != nil {
				return e
			}
			return problem(providers.ErrorUnavailable, "cleanup", "SERVER_STILL_PRESENT", 0)
		})
		if e != nil {
			return e
		}
	}
	return nil
}

var _ providers.AccountResourceCleaner = (*Driver)(nil)

type computeIntent struct {
	Region string `json:"region"`
	Plan   string `json:"plan"`
	Image  string `json:"image"`
}
