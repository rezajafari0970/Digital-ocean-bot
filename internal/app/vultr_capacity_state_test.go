package app

import (
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestVultrCapacityStateMachineThreeThenFive(t *testing.T) {
	state := vultrCapacityState{Phase: vultrCapacityUnknown}
	want := func(phase vultrCapacityPhase, lower, exact int) {
		t.Helper()
		if state.Phase != phase || state.LowerBound != lower || state.ExactLimit != exact {
			t.Fatalf("state=%+v want phase=%s lower=%d exact=%d", state, phase, lower, exact)
		}
	}

	want(vultrCapacityUnknown, 0, 0)
	state = learnVultrSuccess(state, 1)
	want(vultrCapacityLowerBound, 1, 0)
	state = learnVultrSuccess(state, 2)
	want(vultrCapacityLowerBound, 2, 0)
	state = learnVultrSuccess(state, 3)
	want(vultrCapacityLowerBound, 3, 0)
	state = learnVultrSaturation(state, 3)
	want(vultrCapacityExact, 3, 3)

	state.Phase = vultrCapacityProbing
	state = learnVultrSuccess(state, 4)
	state.Phase = vultrCapacityLowerBound
	state.ExactLimit = 0
	want(vultrCapacityLowerBound, 4, 0)
	state = learnVultrSuccess(state, 5)
	want(vultrCapacityLowerBound, 5, 0)
	state = learnVultrSaturation(state, 5)
	want(vultrCapacityExact, 5, 5)
}

func TestDesiredCeilingStopsCapacityDiscovery(t *testing.T) {
	desired := 4
	managed := 3
	preCreate := 0
	if !desiredAllowsCreate(desired, managed, preCreate) {
		t.Fatal("fourth server must be allowed")
	}
	managed = 4
	if desiredAllowsCreate(desired, managed, preCreate) {
		t.Fatal("fifth server must be blocked even when provider capacity is higher")
	}
}

func TestCapacityLearningIsMonotonic(t *testing.T) {
	state := vultrCapacityState{Phase: vultrCapacityLowerBound, LowerBound: 5}
	state = learnVultrSuccess(state, 4)
	if state.LowerBound != 5 {
		t.Fatalf("lower bound regressed: %+v", state)
	}
}

func TestOnlyAccountCapacityErrorProvesSaturation(t *testing.T) {
	cases := []struct {
		class providers.ErrorClass
		want  bool
	}{
		{providers.ErrorCapacity, true},
		{providers.ErrorRegionCapacity, false},
		{providers.ErrorRateLimited, false},
		{providers.ErrorTransport, false},
		{providers.ErrorInvalidRequest, false},
	}
	for _, tc := range cases {
		err := &providers.Error{Class: tc.class, Operation: "create_server"}
		if got := vultrErrorProvesSaturation(err); got != tc.want {
			t.Fatalf("class=%s got=%v want=%v", tc.class, got, tc.want)
		}
	}
}
