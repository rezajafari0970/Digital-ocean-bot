package scheduler

import "time"

type Decision string

const (
	AllowCreate   Decision = "ALLOW_CREATE"
	BlockCapacity Decision = "BLOCK_CAPACITY"
	BlockAccount  Decision = "BLOCK_ACCOUNT"
	BlockNetwork  Decision = "BLOCK_NETWORK"
)

type Capacity struct {
	Limit               int
	Active              int
	Creating            int
	Provisioning        int
	ReservedReplacement int
}

func (c Capacity) Available() int {
	return c.Limit - c.Active - c.Creating - c.Provisioning - c.ReservedReplacement
}

type Policy struct {
	MaxServers int
	Lifetime   time.Duration
}
