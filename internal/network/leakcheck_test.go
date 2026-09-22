package network

import (
	"net/http"
	"testing"
)

func TestLeakObservationPassesExpectedProxy(t *testing.T) {
	o := LeakObservation{ObservedIP: "198.51.100.10", ExpectedExitIP: "198.51.100.10", ServerPublicIP: "203.0.113.9", Headers: make(http.Header)}
	if err := ValidateLeakObservation(o); err != nil {
		t.Fatal(err)
	}
}

func TestLeakObservationRejectsServerIP(t *testing.T) {
	o := LeakObservation{ObservedIP: "203.0.113.9", ExpectedExitIP: "203.0.113.9", ServerPublicIP: "203.0.113.9", Headers: make(http.Header)}
	if err := ValidateLeakObservation(o); err == nil {
		t.Fatal("server IP leak must fail")
	}
}

func TestLeakObservationRejectsForwardingHeader(t *testing.T) {
	h := make(http.Header)
	h.Set("X-Real-IP", "203.0.113.9")
	o := LeakObservation{ObservedIP: "198.51.100.10", ExpectedExitIP: "198.51.100.10", ServerPublicIP: "203.0.113.9", Headers: h}
	if err := ValidateLeakObservation(o); err == nil {
		t.Fatal("leak header must fail")
	}
}
