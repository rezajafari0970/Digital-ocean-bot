package proxycontrol

import (
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/resilience"
)

type State struct {
	AccountID               string
	ProxyID                 string
	Provider                string
	HealthState             network.ProxyStatus
	CircuitState            resilience.CircuitState
	ConsecutiveFailures     int
	ConsecutiveSuccesses    int
	RetryAfter              *time.Time
	Generation              int64
	LastErrorClass          string
	LastErrorDetail         string
	LastCheckedAt           *time.Time
	LastSuccessAt           *time.Time
	HalfOpenProbeInFlight   bool
	HalfOpenProbeLeaseUntil *time.Time
}

func DefaultState(accountID, proxyID, provider string) State {
	return State{
		AccountID:    accountID,
		ProxyID:      proxyID,
		Provider:     provider,
		HealthState:  network.StatusUnknown,
		CircuitState: resilience.Closed,
		Generation:   1,
	}
}
