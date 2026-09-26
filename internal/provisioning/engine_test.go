package provisioning

import (
	"context"
	"testing"
)

type memStore struct {
	r           Run
	exists      bool
	attempts    map[string]int
	interrupted map[string]bool
}

func (s *memStore) Reserve(_ context.Context, r Run) (Run, bool, error) {
	if s.exists {
		return s.r, false, nil
	}
	r.ID = "1"
	s.r = r
	s.exists = true
	return r, true, nil
}
func (s *memStore) Update(_ context.Context, r Run) error { s.r = r; return nil }
func (s *memStore) BeginStep(_ context.Context, _ string, step string, max int) (int, error) {
	if s.attempts == nil {
		s.attempts = map[string]int{}
	}
	if max > 0 && s.attempts[step] >= max {
		return s.attempts[step], ErrStepRetryLimit
	}
	s.attempts[step]++
	return s.attempts[step], nil
}
func (s *memStore) FinishStep(context.Context, string, string, error, bool) error { return nil }
func (s *memStore) StepInterrupted(_ context.Context, _ string, step string) (bool, error) {
	return s.interrupted != nil && s.interrupted[step], nil
}

type secretStub struct{}

func (secretStub) Get(context.Context, string, string) ([]byte, error) { return []byte("key"), nil }

type sshStub struct{ waits, runs int }

func (s *sshStub) Wait(context.Context, Target, []byte) error { s.waits++; return nil }
func (s *sshStub) Run(context.Context, Target, []byte, string) (string, error) {
	s.runs++
	return "", nil
}

func TestProvisionEngineCompletesAndDoesNotRepeat(t *testing.T) {
	store := &memStore{}
	ssh := &sshStub{}
	e := Engine{Store: store, Secrets: secretStub{}, SSH: ssh}
	target := Target{AccountID: "a", DropletID: "d", KeySecretRef: "ssh"}
	plan := Plan{Bootstrap: "a", InstallPanel: "b", Verify: "c"}
	r, err := e.Execute(context.Background(), target, plan)
	if err != nil {
		t.Fatal(err)
	}
	if r.State != Completed || ssh.waits != 1 || ssh.runs != 3 {
		t.Fatalf("bad result %#v waits=%d runs=%d", r, ssh.waits, ssh.runs)
	}
	_, err = e.Execute(context.Background(), target, plan)
	if err != nil {
		t.Fatal(err)
	}
	if ssh.waits != 1 || ssh.runs != 3 {
		t.Fatal("completed provisioning repeated")
	}
}
