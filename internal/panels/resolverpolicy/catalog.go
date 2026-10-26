package resolverpolicy

import _ "embed"

//go:embed catalog.json
var Catalog []byte

// Six unfiltered endpoints across three independent providers.
func Active() []string {
	return []string{"1.1.1.1", "8.8.8.8", "94.140.14.140", "1.0.0.1", "8.8.4.4", "94.140.14.141"}
}

// The legacy plan remains available for exact canary rollback.
func TCPServers() []string { return []string{"tcp://1.1.1.1:53", "tcp://8.8.8.8:53"} }

// A finite native race avoids six serial four-second waits. These transports
// pass through the existing DNS routing policy; no +local/system bypass exists.
func ResilientTCPServers() []any {
	servers := []any{}
	for _, ip := range Active() {
		servers = append(servers, map[string]any{"address": "tcp://" + ip + ":53", "timeoutMs": 2000})
	}
	return servers
}
