package advancedreality

type Capability struct {
	ID     string `json:"id"`
	Layer  string `json:"layer"`
	Status string `json:"status"`
	Note   string `json:"note"`
}

// Source-reviewed possibilities, not proof that a fleet binary implements
// them. Client-version-dependent features never become server controls.
func Capabilities() []Capability {
	return []Capability{
		{"config_contract", "control_plane", "IMPLEMENTED", "Exact JSON shape and immutable-by-default values; dry-run only."},
		{"snapshot_rollback", "lab", "IMPLEMENTED", "Private snapshot lab; CAS, durable idempotency, exact-byte restore and owned-file removal. Not live rollback."},
		{"adaptive_policy", "control_plane", "SHADOW_ONLY", "Hysteresis, freshness, replay rejection and cooldown; recommendations only."},
		{"sni_score", "existing_project", "REUSE_EXISTING", "Use internal/reality scorer and stability; never rotate published SNI or keys automatically."},
		{"tls_reality_validation", "server_core", "NATIVE_TEST_REQUIRED", "Preserve authentication and TLS; native validator diagnostics must be redacted."},
		{"xhttp", "server_core", "VERSION_AND_CLIENT_TEST_REQUIRED", "Exact version modes, limits and padding; preserve the official wire format."},
		{"xmux", "client_config", "CLIENT_ONLY", "Client outbound connection pool; not server stream admission; no Mux.Cool addition."},
		{"tls_record_buffer_flush", "server_core", "CORE_PATCH_RESEARCH", "No generic TLS record-size knob; distinct from HTTP chunks and TCP segments."},
		{"tls13_cipher_selection", "tls_library", "NOT_GENERIC_CONFIG_CONTROL", "TLS<=1.2 cipher settings do not select Go TLS1.3 cipher suites."},
		{"reality_tcp_finalmask", "server_core", "REJECTED_COMBINATION", "Keep the Sanaei REALITY/finalmask.tcp rejection; do not bypass it."},
		{"linux_pacing_cgroups", "server_os", "HOST_PREFLIGHT_REQUIRED", "Inspect kernel, qdisc and service ownership before scoped experiments; no global sysctl writes."},
		{"graceful_live_apply", "panel_and_core", "BLOCKED", "Existing apply can fall back to restart; active-stream preservation is not proven."},
		{"official_v2rayng", "client", "UNVERIFIED", "No client binary changes. Exact installed core and real import/connection evidence required."},
		{"dpi_invisibility", "network", "NOT_GUARANTEED", "Measure connection success and traffic properties from external networks; no universal invisibility claim."},
	}
}

func Unmeasured() []string {
	return []string{
		"tls_handshake_success_rate", "tls_error_rate", "reality_authentication_error_rate", "xhttp_error_rate",
		"client_connection_success_rate", "active_connections", "idle_connections", "concurrent_streams",
		"request_rate", "response_rate", "path_rtt_p95", "packet_loss", "retransmission_rate",
		"timeout_rate", "reset_rate", "buffer_bytes", "queue_depth", "gc_pause", "service_restart_count",
	}
}
