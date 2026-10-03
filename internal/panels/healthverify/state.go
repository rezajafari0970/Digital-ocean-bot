package healthverify

const UnhealthyFailureThreshold = 3

func stateAfterFailure(failures int) string {
	if failures >= UnhealthyFailureThreshold {
		return "UNHEALTHY"
	}
	return "DEGRADED"
}
