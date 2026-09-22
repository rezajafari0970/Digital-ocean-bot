package testharness

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"sync"
)

var ErrInjected = errors.New("injected failure")

type FailurePlan struct {
	Step  string
	Times int
}
type Steps struct {
	mu              sync.Mutex
	Calls           map[string]int
	Failure         FailurePlan
	ProviderCreates int
	ProviderDeletes int
}

func NewSteps(f FailurePlan) *Steps { return &Steps{Calls: map[string]int{}, Failure: f} }
func (s *Steps) call(name string, d workflow.Deployment) (workflow.Deployment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls[name]++
	if s.Failure.Step == name && s.Calls[name] <= s.Failure.Times {
		return d, ErrInjected
	}
	if name == "create" {
		s.ProviderCreates++
		d.ProviderID = "42"
	}
	if name == "wait_resource" {
		d.DropletID = "droplet-1"
	}
	return d, nil
}
func (s *Steps) Create(_ context.Context, d workflow.Deployment) (workflow.Deployment, error) {
	return s.call("create", d)
}
func (s *Steps) WaitResource(_ context.Context, d workflow.Deployment) (workflow.Deployment, error) {
	return s.call("wait_resource", d)
}
func (s *Steps) Provision(_ context.Context, d workflow.Deployment) (workflow.Deployment, error) {
	return s.call("provision", d)
}
func (s *Steps) ImportDatabase(_ context.Context, d workflow.Deployment) (workflow.Deployment, error) {
	return s.call("database", d)
}
func (s *Steps) ConfigurePanel(_ context.Context, d workflow.Deployment) (workflow.Deployment, error) {
	return s.call("panel", d)
}
func (s *Steps) RegisterClients(_ context.Context, d workflow.Deployment, _ workflow.Request) (workflow.Deployment, error) {
	return s.call("clients", d)
}
func (s *Steps) RegisterTraffic(_ context.Context, d workflow.Deployment) (workflow.Deployment, error) {
	return s.call("traffic", d)
}
