package resolverpolicy

import _ "embed"

//go:embed catalog.json
var Catalog []byte

// Three verified, unfiltered IPv4 resolvers bound retry fanout. Queries follow
// Xray routing, including the residential fail-closed DNS rule.
func Active() []string { return []string{"1.1.1.1", "8.8.8.8", "223.5.5.5"} }
