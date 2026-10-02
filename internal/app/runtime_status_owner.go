package app

func networkMaySetReady(status string) bool {
	switch status {
	case "", "READY", "ISOLATION_WAIT", "PROVIDER_PROXY_ERROR", "PROVIDER_TRANSPORT_ERROR":
		return true
	default:
		return false
	}
}
