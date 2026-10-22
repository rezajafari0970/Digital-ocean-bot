package droplets

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/jobs"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"sync/atomic"
	"testing"
)

type createAtomicProvider struct {
	computeStub
	calls atomic.Int32
	check func(providers.CreateServerRequest) error
}

func (p *createAtomicProvider) CreateServer(_ context.Context, r providers.CreateServerRequest) (providers.CreateServerResult, error) {
	p.calls.Add(1)
	if e := p.check(r); e != nil {
		return providers.CreateServerResult{}, e
	}
	return providers.CreateServerResult{ServerID: "native-server", Outcome: providers.OutcomeAccepted}, nil
}
func TestCreateResumeConcurrentPostgres(t *testing.T) {
	db := deleteResumeDB(t)
	ctx := context.Background()
	var account string
	if e := db.QueryRow("INSERT INTO accounts(id,provider,name,secret_ref) VALUES(gen_random_uuid(),'aws','create-resume-fixture','fixture') RETURNING id::text").Scan(&account); e != nil {
		t.Fatal(e)
	}
	profile := Profile{Name: "fixed", Region: "r2", Size: "semantic-plan", Image: "semantic-image", IdentityTag: "durable-identity", RegionalLaunches: map[string]providers.RegionalLaunch{"r2": {PlanID: "native-plan-r2", ImageID: "native-image-r2"}}}
	op := BuildCreateOperation(account, profile)
	store := jobs.SQLStore{DB: db}
	if _, _, e := store.Reserve(ctx, op); e != nil {
		t.Fatal(e)
	}
	const n = 12
	barrier := &deleteReserveBarrier{Store: store}
	barrier.arrived.Add(n)
	driver := &createAtomicProvider{check: func(r providers.CreateServerRequest) error {
		current, e := store.Get(ctx, account, op.IdempotencyKey)
		if e != nil {
			return e
		}
		if current.State != jobs.OperationRunning || current.Attempt != 1 {
			return errors.New("provider called before durable claim")
		}
		if r.PlanID != "native-plan-r2" || r.ImageID != "native-image-r2" || r.Identity != profile.IdentityTag || r.RegionID != "r2" {
			return errors.New("regional launch identity changed")
		}
		return nil
	}}
	executor := Executor{Operations: barrier, Provider: driver, Gate: gateStub{}}
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() { _, e := executor.Create(ctx, op, profile); results <- e }()
	}
	success := 0
	for i := 0; i < n; i++ {
		e := <-results
		if e == nil {
			success++
		} else if !errors.Is(e, jobs.ErrOperationVersionConflict) {
			t.Fatal(e)
		}
	}
	current, e := store.Get(ctx, account, op.IdempotencyKey)
	if e != nil || success != 1 || driver.calls.Load() != 1 || current.Attempt != 1 || current.State != jobs.OperationVerifying || current.ResourceID != "native-server" {
		t.Fatal("concurrent create replay", success, driver.calls.Load(), current, e)
	}
	executor.Operations = store
	if _, e = executor.Create(ctx, op, profile); e != nil || driver.calls.Load() != 1 {
		t.Fatal("verifying create replayed", e)
	}
}
