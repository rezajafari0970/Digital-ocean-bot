package inventory

import "time"

type InboundRecord struct {
	PanelID string

	RemoteID int64

	Remark   string
	Protocol string

	Port   int
	Listen string

	Enabled bool

	Transport string
	Security  string

	ClientCount int

	Upload   int64
	Download int64
	Total    int64

	// Hash of the complete raw inbound object.
	//
	// The raw object itself may contain client UUIDs
	// or other sensitive values, so it is not stored
	// in the normalized inventory.
	RawHash string

	ObservedAt time.Time
}

type Snapshot struct {
	PanelID string

	ObservedAt time.Time

	Records []InboundRecord
}
