package testharness

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/workflow"
	"sync"
)

type Store struct {
	mu     sync.Mutex
	D      workflow.Deployment
	Exists bool
	Events []string
}

func (s *Store) Reserve(_ context.Context, r workflow.Request) (workflow.Deployment, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Exists {
		return s.D, false, nil
	}
	s.D = workflow.Deployment{ID: "deployment-1", AccountID: r.AccountID, ProfileID: r.ProfileID, State: workflow.Planned, CurrentStep: "create"}
	s.Exists = true
	return s.D, true, nil
}
func (s *Store) Update(_ context.Context, d workflow.Deployment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.D = d
	return nil
}
func (s *Store) Event(_ context.Context, _ string, step string, state workflow.State, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Events = append(s.Events, step+":"+string(state))
	return nil
}
