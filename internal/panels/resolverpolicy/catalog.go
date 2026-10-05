package resolverpolicy

import _ "embed"

//go:embed catalog.json
var Catalog []byte

// Two independent unfiltered IPv4 providers bound retry fanout.
// Managed category-only DNS follows the server-direct routing rule.
func Active() []string { return []string{"1.1.1.1", "8.8.8.8"} }

// TCP DNS still traverses Xray routing. Never use tcp+local, which bypasses it.
func TCPServers() []string {
	result := []string{}
	for _, address := range Active() {
		result = append(result, "tcp://"+address+":53")
	}
	return result
}
