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
		a.Flow == b.Flow
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
	switch job.Kind {
	case KindDelete:
		return !exists, nil
	case KindCreate:
		if !exists {
			return false, nil
		}
		want, err := payloadClient(job.Payload)
		if err != nil {
			return false, err
		}
		if !sameClient(current, want) {
			return true, ErrClientConflict
		}
		return true, nil
	case KindUpdate:
		return false, ErrUnsupportedKind
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
	return rt.WithMutation(ctx, func(runCtx context.Context) error {
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
			err = sanaei.AddClientsSession(runCtx, rt.Session.Exec, int(job.InboundID), []sanaei.Client{client})
		case KindDelete:
			err = sanaei.DeleteClientSession(runCtx, rt.Session.Exec, int(job.InboundID), job.ClientID)
		case KindUpdate:
			return ErrUnsupportedKind
		default:
			return ErrUnsupportedKind
		}

		rt.Session.Invalidate()
		verified, verifyErr := e.desiredSatisfied(runCtx, rt, job)
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
	})
}

func (e Executor) RunOne(ctx context.Context) (bool, error) {
	if e.Journal.DB == nil || e.Runtimes == nil {
		return false, ErrInvalidRequest
	}
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
	case execErr == nil:
		return true, e.Journal.Succeed(finishCtx, job.ID)
	case errors.Is(execErr, ErrUnsupportedKind),
		errors.Is(execErr, ErrClientConflict),
		errors.Is(execErr, ErrInvalidRequest):
		return true, e.Journal.Fail(finishCtx, job.ID, execErr.Error())
	case errors.Is(execErr, ErrInboundMissing):
		return true, e.Journal.Obsolete(finishCtx, job.ID, execErr.Error())
	case job.Attempts >= 3:
		return true, e.Journal.Fail(finishCtx, job.ID, execErr.Error())
	default:
		delay := time.Duration(job.Attempts*2) * time.Second
		if delay > 10*time.Second {
			delay = 10 * time.Second
		}
		if err := e.Journal.Retry(finishCtx, job.ID, execErr.Error(), delay); err != nil {
			return true, fmt.Errorf("retry client mutation: %w", err)
		}
		return true, nil
	}
}
