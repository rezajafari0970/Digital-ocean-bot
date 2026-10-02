package app

import (
	"testing"
	"time"
)

func TestProviderRefreshSkipHonorsExplicitInvalidation(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	if !providerRefreshMaySkip(true, ProviderStateActive, "", now.Add(-30*time.Second), 90*time.Second, now) {
		t.Fatal("fresh snapshot with fresh provider check should skip")
	}
	if providerRefreshMaySkip(true, ProviderStateActive, "", time.Time{}, 90*time.Second, now) {
		t.Fatal("zero provider_checked_at must force refresh")
	}
	if providerRefreshMaySkip(true, ProviderStateActive, ProviderStateTransportError, now, 90*time.Second, now) {
		t.Fatal("provider error must force refresh")
	}
	if providerRefreshMaySkip(true, ProviderStateActive, "", now.Add(-2*time.Minute), 90*time.Second, now) {
		t.Fatal("stale provider check must force refresh")
	}
}
