package policy

type InboundPolicy struct {
	ID                string
	PanelID           string
	Enabled           bool
	DesiredCount      int
	Protocol          string
	Transport         string
	Security          string
	Listen            string
	PreferredPorts    []int
	DynamicPortStart  int
	DynamicPortEnd    int
	ReservedPorts     []int
	ClientsPerInbound int
	AllowDelete       bool
}
