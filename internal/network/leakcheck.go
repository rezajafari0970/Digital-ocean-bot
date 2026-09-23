package network

import (
	"errors"
	"net"
	"net/http"
)

var (
	ErrUnexpectedExitIP = errors.New("unexpected proxy exit ip")
	ErrServerIPLeak     = errors.New("server public ip leaked")
)

type LeakObservation struct {
	ObservedIP     string
	ExpectedExitIP string
	ServerPublicIP string
	Headers        http.Header
}

func ValidateLeakObservation(o LeakObservation) error {
	observed := net.ParseIP(o.ObservedIP)
	if observed == nil {
		return ErrUnexpectedExitIP
	}
	if o.ExpectedExitIP != "" {
		expected := net.ParseIP(o.ExpectedExitIP)
		if expected == nil || !observed.Equal(expected) {
			return ErrUnexpectedExitIP
		}
	}
	server := net.ParseIP(o.ServerPublicIP)
	if server != nil && observed.Equal(server) {
		return ErrServerIPLeak
	}
	if HasForbiddenOutboundHeader(o.Headers) {
		return ErrForbiddenOutboundHeader
	}
	return nil
}
