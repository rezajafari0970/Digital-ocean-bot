package providers

const UpCloudTrialPanelPort = 3389

// A permitted proxy service port is not proof of endpoint health.
func UpCloudTrialProxyPortAllowed(port int) bool {
	return port == 80 || port == 443 || port == 8080
}
func UpCloudTrialClientPortAllowed(port int) bool {
	return port == 80 || port == 443
}
