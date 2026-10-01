package app

import "github.com/rezajafari0970/Digital-ocean-bot/internal/providers"

type vultrCapacityPhase string

const (
	vultrCapacityUnknown    vultrCapacityPhase = "UNKNOWN"
	vultrCapacityLowerBound vultrCapacityPhase = "LOWER_BOUND"
	vultrCapacityExact      vultrCapacityPhase = "EXACT"
	vultrCapacityProbing    vultrCapacityPhase = "PROBING"
)

type vultrCapacityState struct {
	Phase      vultrCapacityPhase
	LowerBound int
	ExactLimit int
}

func learnVultrSuccess(s vultrCapacityState, observed int) vultrCapacityState {
	if observed > s.LowerBound {
		s.LowerBound = observed
	}
	if s.Phase != vultrCapacityProbing {
		s.Phase = vultrCapacityLowerBound
		s.ExactLimit = 0
	}
	return s
}

func learnVultrSaturation(s vultrCapacityState, current int) vultrCapacityState {
	if current > s.LowerBound {
		s.LowerBound = current
	}
	s.Phase = vultrCapacityExact
	s.ExactLimit = current
	return s
}

func vultrErrorProvesSaturation(err error) bool {
	return providers.IsClass(err, providers.ErrorCapacity)
}
