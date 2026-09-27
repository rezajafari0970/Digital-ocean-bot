package planner

type DesiredInbound struct {
	Key       string
	Remark    string
	Protocol  string
	Port      int
	Listen    string
	Enabled   bool
	Transport string
	Security  string
	Managed   bool
}

type ActionKind string

const (
	ActionCreate ActionKind = "CREATE"
	ActionUpdate ActionKind = "UPDATE"
	ActionDelete ActionKind = "DELETE"
	ActionNoop   ActionKind = "NOOP"
)

type Action struct {
	Kind     ActionKind
	Key      string
	RemoteID int64
	Desired  DesiredInbound
	Reason   string
}

type Policy struct {
	AllowDelete bool
}
