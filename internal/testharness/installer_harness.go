package testharness

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

type InstallerStore struct {
	mu       sync.Mutex
	Attempts map[string]int
	Started  map[string]time.Time
	Finished map[string]time.Time
	LastErr  map[string]error
	Terminal map[string]bool
	Events   []provisioning.Event
}

func NewInstallerStore() *InstallerStore {
	return &InstallerStore{Attempts: map[string]int{}, Started: map[string]time.Time{}, Finished: map[string]time.Time{}, LastErr: map[string]error{}, Terminal: map[string]bool{}}
}
func (s *InstallerStore) BeginStep(_ context.Context, _, step string, max int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if max > 0 && s.Attempts[step] >= max {
		return s.Attempts[step], provisioning.ErrStepRetryLimit
	}
	s.Attempts[step]++
	s.Started[step] = time.Now()
	return s.Attempts[step], nil
}
func (s *InstallerStore) FinishStep(_ context.Context, _, step string, err error, terminal bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Finished[step] = time.Now()
	s.LastErr[step] = err
	s.Terminal[step] = terminal
	return nil
}
func (s *InstallerStore) StepCompleted(_ context.Context, _, step string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.Started[step]
	if !ok {
		return false, nil
	}
	fin, ok := s.Finished[step]
	if !ok || fin.Before(st) || s.LastErr[step] != nil || s.Terminal[step] {
		return false, nil
	}
	return true, nil
}
func (s *InstallerStore) Event(_ context.Context, e provisioning.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Events = append(s.Events, e)
	return nil
}
func (s *InstallerStore) Interrupt(step string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Started[step] = time.Now().Add(time.Second)
	delete(s.Finished, step)
	delete(s.LastErr, step)
	s.Terminal[step] = false
}

type InstallerStates struct {
	mu     sync.Mutex
	States []string
}

func (s *InstallerStates) SetState(_ context.Context, _, state, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.States = append(s.States, state)
	return nil
}
func (s *InstallerStates) Last() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.States) == 0 {
		return ""
	}
	return s.States[len(s.States)-1]
}

type InstallerFault string

const (
	InstallerSuccess         InstallerFault = "success"
	InstallerRetryable       InstallerFault = "retryable"
	InstallerTerminal        InstallerFault = "terminal"
	InstallerVerifyFailure   InstallerFault = "verify_failure"
	InstallerRollbackFailure InstallerFault = "rollback_failure"
)

type InstallerScripts struct {
	mu            sync.Mutex
	Fault         InstallerFault
	Calls         map[string]int
	Marker        bool
	RetryInjected bool
	Writes        int
}

func NewInstallerScripts(f InstallerFault) *InstallerScripts {
	return &InstallerScripts{Fault: f, Calls: map[string]int{}}
}
func (s *InstallerScripts) RunScript(_ context.Context, _ provisioning.Target, _ []byte, step provisioning.ScriptStep, observe func(provisioning.ScriptPhaseResult), _ provisioning.StageObserver) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls[step.Name]++
	emit := func(phase string, r provisioning.CommandResult, err error) {
		if observe != nil {
			observe(provisioning.ScriptPhaseResult{Phase: phase, Result: r, Err: err})
		}
	}
	if step.Category == "rollback" {
		if s.Fault == InstallerRollbackFailure {
			code := 1
			err := errors.Join(provisioning.ErrSSHCommand, errors.New("permission denied"))
			r := provisioning.CommandResult{Stderr: "permission denied", ExitCode: &code}
			emit("execute", r, err)
			return err
		}
		s.Marker = false
		emit("execute", provisioning.CommandResult{}, nil)
		emit("verify", provisioning.CommandResult{}, nil)
		return nil
	}
	if step.Category == "verify" {
		if s.Fault == InstallerVerifyFailure {
			code := 1
			err := errors.Join(provisioning.ErrSSHCommand, errors.New("permission denied"))
			r := provisioning.CommandResult{Stderr: "permission denied", ExitCode: &code}
			emit("verify", r, err)
			return err
		}
		if !s.Marker {
			code := 1
			err := errors.Join(provisioning.ErrSSHCommand, errors.New("marker missing"))
			r := provisioning.CommandResult{Stderr: "marker missing", ExitCode: &code}
			emit("verify", r, err)
			return err
		}
		emit("verify", provisioning.CommandResult{}, nil)
		return nil
	}
	if s.Marker {
		emit("precheck", provisioning.CommandResult{}, nil)
		return nil
	}
	code := 1
	emit("precheck", provisioning.CommandResult{ExitCode: &code}, provisioning.ErrSSHCommand)
	if s.Fault == InstallerRetryable && !s.RetryInjected {
		s.RetryInjected = true
		err := errors.Join(provisioning.ErrSSHCommand, errors.New("temporary failure resolving host"))
		r := provisioning.CommandResult{Stderr: "temporary failure resolving host", ExitCode: &code}
		emit("execute", r, err)
		return err
	}
	if s.Fault == InstallerTerminal {
		err := errors.Join(provisioning.ErrSSHCommand, errors.New("permission denied"))
		r := provisioning.CommandResult{Stderr: "permission denied", ExitCode: &code}
		emit("execute", r, err)
		return err
	}
	s.Marker = true
	s.Writes++
	emit("execute", provisioning.CommandResult{}, nil)
	emit("verify", provisioning.CommandResult{}, nil)
	return nil
}

func InstallerFixture(f InstallerFault) (provisioning.InstallerExecutor, *InstallerStore, *InstallerStates, *InstallerScripts, provisioning.ResolvedInstaller, provisioning.InstallerRun) {
	store := NewInstallerStore()
	states := &InstallerStates{}
	scripts := NewInstallerScripts(f)
	resolved := provisioning.ResolvedInstaller{Manifest: provisioning.InstallerManifest{Name: "harness", Version: 1}, Steps: []provisioning.ScriptStep{{Name: "install", Category: "install", Precheck: "check", Execute: "install", Verify: "verify", MaxAttempts: 3}, {Name: "verify", Category: "verify", Verify: "verify", MaxAttempts: 2}}, Rollback: []provisioning.ScriptStep{{Name: "rollback", Category: "rollback", Precheck: "check", Execute: "undo", Verify: "verify", MaxAttempts: 2}}}
	run := provisioning.InstallerRun{ID: "installer-run", ProvisionRunID: "provision-run", Generation: 1}
	exec := provisioning.InstallerExecutor{Store: store, Events: store, Scripts: scripts, States: states}
	return exec, store, states, scripts, resolved, run
}

func (s *InstallerStore) Reserve(_ context.Context, r provisioning.Run) (provisioning.Run, bool, error) {
	return r, true, nil
}
func (s *InstallerStore) Update(_ context.Context, _ provisioning.Run) error { return nil }
func (s *InstallerStore) StepInterrupted(_ context.Context, _, step string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.Started[step]
	if !ok {
		return false, nil
	}
	fin, ok := s.Finished[step]
	return !ok || fin.Before(st), nil
}
