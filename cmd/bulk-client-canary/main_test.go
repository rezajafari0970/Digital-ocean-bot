package main

import "testing"

func TestCanaryCountCannotSkipAcceptedRamp(t *testing.T) {
	for _, count := range []int{-1, 0, 1, 24, 26, 49, 51, 100, 250, 10000} {
		if validateCanaryCount(count) == nil {
			t.Fatalf("unsafe ramp accepted: %d", count)
		}
	}
	for _, count := range []int{10, 25, 50} {
		if err := validateCanaryCount(count); err != nil {
			t.Fatal(err)
		}
	}
}
