package app

import "testing"

func TestDeleteFirstAtHardDesiredPolicy(t *testing.T) {
	tests := []struct {
		name                       string
		desired, managed, retiring int
		oldest, want               bool
	}{
		{"at desired oldest", 15, 15, 0, true, true},
		{"above desired oldest", 15, 16, 0, true, true},
		{"below desired", 15, 14, 0, true, false},
		{"another retirement active", 15, 15, 1, true, false},
		{"not oldest", 15, 15, 0, false, false},
		{"zero desired fail closed", 0, 15, 0, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldDeleteFirstAtDesired(tt.desired, tt.managed, tt.retiring, tt.oldest); got != tt.want {
				t.Fatalf("got=%v want=%v", got, tt.want)
			}
		})
	}
}

func TestDeficitBelongsToSchedulerBackfill(t *testing.T) {
	if !shouldWaitForDeficitBackfill(17, 16) {
		t.Fatal("lifecycle must not claim a scheduler/backfill deficit as an unrelated replacement")
	}
	if shouldWaitForDeficitBackfill(17, 17) {
		t.Fatal("at desired is not a deficit")
	}
	if shouldWaitForDeficitBackfill(17, 18) {
		t.Fatal("oversupply is not a deficit")
	}
	if shouldWaitForDeficitBackfill(0, 0) {
		t.Fatal("zero desired must fail closed")
	}
}
