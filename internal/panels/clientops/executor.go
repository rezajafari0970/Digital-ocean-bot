package clientops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
)

var (
	ErrUnsupportedKind = errors.New("client mutation kind not enabled")
	ErrInboundMissing  = errors.New("client mutation inbound missing")
	ErrClientConflict  = errors.New("client mutation client conflict")
	ErrVerify          = errors.New("client mutation verification failed")
)

type Executor struct {
	Journal  Journal
	Runtimes *sanaei.RuntimeManager
	Timeout  time.Duration
}

func (e Executor) timeout() time.Duration {
	if e.Timeout <= 0 {
		return 30 * time.Second
	}
	return e.Timeout
}

func payloadClient(raw json.RawMessage) (sanaei.Client, error) {
	var envelope struct {
		Client sanaei.Client
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return sanaei.Client{}, err
	}
	if envelope.Client.ID == "" {
		return sanaei.Client{}, ErrInvalidRequest
	}
	return envelope.Client, nil
}

func clientFromInbound(raw json.RawMessage, clientID string) (sanaei.Client, bool, error) {
	var in struct {
		Settings any `json:"settings"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return sanaei.Client{}, false, err
	}
	var settingsBytes []byte
	switch v := in.Settings.(type) {
	case string:
		settingsBytes = []byte(v)
	case map[string]any:
		var err error
		settingsBytes, err = json.Marshal(v)
		if err != nil {
			return sanaei.Client{}, false, err
		}
	default:
		return sanaei.Client{}, false, ErrInboundMissing
	}
	var settings struct {
		Clients []sanaei.Client `json:"clients"`
	}
	if err := json.Unmarshal(settingsBytes, &settings); err != nil {
		return sanaei.Client{}, false, err
	}
	for _, c := range settings.Clients {
		if c.ID == clientID {
			return c, true, nil
		}
	}
	return sanaei.Client{}, false, nil
}

func sameClient(a, b sanaei.Client) bool {
	return a.ID == b.ID &&
		a.Email == b.Email &&
		a.Enable == b.Enable &&
		a.TotalGB == b.TotalGB &&
		a.ExpiryTime == b.ExpiryTime &&
		a.LimitIP == b.LimitIP &&
		a.LimitHWID == b.LimitHWID &&
		a.Flow == b.Flow
}

type observedDecision int

const (
	decisionMutate observedDecision = iota
	decisionSatisfied
	decisionConflict
)

func decideObserved(kind Kind, exists bool, current, wanted sanaei.Client) observedDecision {
	switch kind {
	case KindDelete:
		if !exists {
			return decisionSatisfied
		}
		return decisionMutate
	case KindCreate:
		if !exists {
			return decisionMutate
		}
		if sameClient(current, wanted) {
			return decisionSatisfied
		}
		return decisionConflict
	default:
		return decisionConflict
	}
}

func (e Executor) desiredSatisfied(ctx context.Context, rt *sanaei.PanelRuntime, job Job) (bool, error) {
	rt.Session.Invalidate()
	raw, found, err := rt.Session.RawInbound(ctx, job.InboundID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, ErrInboundMissing
	}
	current, exists, err := clientFromInbound(raw, job.ClientID)
	if err != nil {
		return false, err
	}
	if job.Kind == KindUpdate {
		if !exists {
			return true, ErrClientConflict
		}
		patch, err := payloadPatch(job.Payload)
		if err != nil {
			return false, err
		}
		if patch.Email != nil {
			return false, ErrUnsupportedKind
		}
		return updateSatisfiedEverywhere(ctx, rt, job.ClientID, patch)
	}
	var want sanaei.Client
	if job.Kind == KindCreate {
		want, err = payloadClient(job.Payload)
		if err != nil {
			return false, err
		}
	}
	switch decideObserved(job.Kind, exists, current, want) {
	case decisionSatisfied:
		return true, nil
	case decisionMutate:
		return false, nil
	case decisionConflict:
		if job.Kind == KindUpdate {
			return false, ErrUnsupportedKind
		}
		return true, ErrClientConflict
	default:
		return false, ErrUnsupportedKind
	}
}

func (e Executor) execute(ctx context.Context, job Job) error {
	if e.Runtimes == nil {
		return ErrInvalidRequest
	}
	rt, err := e.Runtimes.Acquire(ctx, job.PanelID)
	if err != nil {
		return err
	}
	return rt.WithMutation(ctx, func(c context.Context) error { return e.executeRuntime(c, rt, job) })
}

// executeRuntime is called with the runtime mutation lock held by execute.
func (e Executor) executeRuntime(ctx context.Context, rt *sanaei.PanelRuntime, job Job) error {
	runCtx := ctx
	if job.Kind == KindBulkDelete {
		return e.executeBulkDelete(runCtx, rt, job)
	}
	if job.Kind == KindBulkCreate {
		return e.executeBulk(runCtx, rt, job)
	}

	if isLifecycle(job.Payload) {
		if job.Kind != KindUpdate {
			return ErrInvalidRequest
		}
		var p struct{ GenerationID, ExpectedEmail string }
		if json.Unmarshal(job.Payload, &p) != nil {
			return ErrInvalidRequest
		}
		release, e := e.lifecycleFence(runCtx, job, p.GenerationID, true)
		if e != nil {
			return e
		}
		defer release()
		records, e := globalWanted(runCtx, rt, job.InboundID, []sanaei.Client{{ID: job.ClientID, Email: p.ExpectedEmail}})
		if e != nil {
			return e
		}
		if _, ok := records[job.ClientID]; !ok {
			return ErrClientConflict
		}
	}
	ok, err := e.desiredSatisfied(runCtx, rt, job)
	if err != nil && !errors.Is(err, ErrUnsupportedKind) {
		return err
	}
	if ok {
		return nil
	}
	switch job.Kind {
	case KindCreate:
		client, err := payloadClient(job.Payload)
		if err != nil {
			return err
		}
		if client.ID != job.ClientID {
			return ErrInvalidRequest
		}
		err = sanaei.AddClientCompatibleSession(runCtx, rt.Session.Exec, int(job.InboundID), client)
	case KindDelete:
		rt.Session.Invalidate()
		raw, found, readErr := rt.Session.RawInbound(runCtx, job.InboundID)
		if readErr != nil {
			return readErr
		}
		if !found {
			return ErrInboundMissing
		}
		current, exists, readErr := clientFromInbound(raw, job.ClientID)
		if readErr != nil {
			return readErr
		}
		if !exists {
			return nil
		}
		if current.Email == "" {
			return ErrClientConflict
		}
		err = sanaei.DeleteClientCompatibleSession(runCtx, rt.Session.Exec, int(job.InboundID), job.ClientID, current.Email)
	case KindUpdate:
		patch, patchErr := payloadPatch(job.Payload)
		if patchErr != nil {
			return patchErr
		}
		if patch.Email != nil {
			return ErrUnsupportedKind
		}
		rt.Session.Invalidate()
		raw, found, readErr := rt.Session.RawInbound(runCtx, job.InboundID)
		if readErr != nil {
			return readErr
		}
		if !found {
			return ErrInboundMissing
		}
		currentMap, exists, readErr := clientMapFromInbound(raw, job.ClientID)
		if readErr != nil {
			return readErr
		}
		if !exists {
			return ErrClientConflict
		}
		currentEmail, _ := currentMap["email"].(string)
		if currentEmail == "" {
			return ErrClientConflict
		}
		global, readErr := sanaei.GetClientByEmailSession(runCtx, rt.Session.Exec, currentEmail)
		if readErr != nil {
			return readErr
		}
		if uuid, _ := global["uuid"].(string); uuid != "" && uuid != job.ClientID {
			return ErrClientConflict
		}
		payload, patchErr := v3UpdatePayload(global, patch)
		if patchErr != nil {
			return patchErr
		}
		err = sanaei.UpdateClientByEmailSession(runCtx, rt.Session.Exec, currentEmail, payload)
	default:
		return ErrUnsupportedKind
	}

	rt.Session.Invalidate()

	verifyCtx := runCtx
	if isLifecycle(job.Payload) {
		var cancel context.CancelFunc
		verifyCtx, cancel = context.WithTimeout(context.WithoutCancel(runCtx), 45*time.Second)
		defer cancel()
	}
	verified, verifyErr := e.desiredSatisfied(verifyCtx, rt, job)
	if verifyErr == nil && verified {
		return nil
	}
	if err != nil {
		return err
	}
	if verifyErr != nil {
		return verifyErr
	}
	return ErrVerify
}

func (e Executor) RunOne(ctx context.Context) (bool, error) {
	if e.Journal.DB == nil || e.Runtimes == nil {
		return false, ErrInvalidRequest
	}
	unlock, acquired, err := e.Journal.executorLock(ctx)
	if err != nil || !acquired {
		return false, err
	}
	defer unlock()
	if err := e.Journal.Reconcile(ctx); err != nil {
		return false, err
	}
	job, ok, err := e.Journal.Claim(ctx)
	if err != nil || !ok {
		return ok, err
	}

	runCtx, cancel := context.WithTimeout(ctx, e.timeout())
	defer cancel()
	execErr := e.execute(runCtx, job)

	finishCtx := context.WithoutCancel(ctx)
	switch {
	case errors.Is(execErr, ErrLifecycleSuperseded):
		return true, e.Journal.supersedeLifecycle(finishCtx, job)
	case execErr == nil:
		return true, e.Journal.Succeed(finishCtx, job.ID)
	case errors.Is(execErr, ErrUnsupportedKind),
		errors.Is(execErr, ErrClientConflict),
		errors.Is(execErr, ErrInvalidRequest):
		return true, retryResult(execErr, e.Journal.Fail(finishCtx, job.ID, execErr.Error()))
	case errors.Is(execErr, ErrInboundMissing):
		return true, retryResult(execErr, e.Journal.Obsolete(finishCtx, job.ID, execErr.Error()))
	case job.Attempts >= 3:
		return true, retryResult(execErr, e.Journal.Fail(finishCtx, job.ID, execErr.Error()))
	default:
		delay := time.Duration(job.Attempts*2) * time.Second
		if delay > 10*time.Second {
			delay = 10 * time.Second
		}
		return true, retryResult(execErr, e.Journal.Retry(finishCtx, job.ID, execErr.Error(), delay))
	}
}

func retryResult(execErr, journalErr error) error {
	if journalErr != nil {
		return fmt.Errorf("retry client mutation: %w", journalErr)
	}
	return execErr
}

func updateSatisfiedEverywhere(ctx context.Context, rt *sanaei.PanelRuntime, clientID string, patch ClientPatch) (bool, error) {
	rt.Session.Invalidate()
	raws, err := rt.Session.Snapshot(ctx)
	if err != nil {
		return false, err
	}
	found := false
	currentEmail := ""
	runtimePatch := patch
	runtimePatch.LimitHWID = nil
	for _, raw := range raws {
		m, ok, e := clientMapFromInbound(raw, clientID)
		if e != nil && !errors.Is(e, ErrClientConflict) {
			return false, e
		}
		if !ok {
			continue
		}
		found = true
		if currentEmail == "" {
			currentEmail, _ = m["email"].(string)
		}
		if !mapPatchSatisfied(m, runtimePatch) {
			return false, nil
		}
	}
	if !found || currentEmail == "" {
		return false, ErrClientConflict
	}
	global, err := sanaei.GetClientByEmailSession(ctx, rt.Session.Exec, currentEmail)
	if err != nil {
		return false, err
	}
	return mapPatchSatisfied(global, patch), nil
}
