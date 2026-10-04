package resolverpolicy

import _ "embed"

//go:embed catalog.json
var Catalog []byte

// Three verified, unfiltered IPv4 resolvers bound retry fanout. Queries follow
// Xray routing, including the residential fail-closed DNS rule.
func Active() []string { return []string{"1.1.1.1", "8.8.8.8", "223.5.5.5"} }

// TCP DNS still traverses Xray routing. Never use tcp+local, which bypasses it.
func TCPServers() []string {
	result := []string{}
	for _, address := range Active() {
		result = append(result, "tcp://"+address+":53")
	}
	return result
}
