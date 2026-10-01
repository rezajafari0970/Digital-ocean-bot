package adminapi

import "time"

type capacityEvidence struct {
	State         string
	LowerBound    int
	Source        string
	ProbeInFlight bool
	ProbeAfter    *time.Time
}

func deriveCapacityEvidence(source string, lowerBound int, probeInFlight bool, probeAfter *time.Time) capacityEvidence {
	state := "UNKNOWN"
	switch {
	case probeInFlight || source == "vultr_api_probe":
		state = "PROBING"
	case source == "vultr_api_saturation" || source == "vultr_console":
		state = "EXACT"
	case lowerBound > 0 || source == "vultr_api_lower_bound" || source == "vultr_api_probe_success":
		state = "PROVEN_LOWER_BOUND"
	}
	return capacityEvidence{State: state, LowerBound: lowerBound, Source: source, ProbeInFlight: probeInFlight, ProbeAfter: probeAfter}
}

func (s *Server) capacityEvidenceForAccount(accountID, provider string) capacityEvidence {
	if provider != "vultr" {
		return capacityEvidence{State: "EXACT"}
	}
	var source string
	var lowerBound int
	var probeInFlight bool
	var probeAfter *time.Time
	err := s.DB.QueryRow(`SELECT source,lower_bound,probe_in_flight,probe_after
		FROM provider_capacity_observations WHERE account_id=$1`, accountID).
		Scan(&source, &lowerBound, &probeInFlight, &probeAfter)
	if err != nil {
		return capacityEvidence{State: "UNKNOWN"}
	}
	return deriveCapacityEvidence(source, lowerBound, probeInFlight, probeAfter)
}
