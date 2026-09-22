package trafficguard

import "time"

type Action string

const (
	ActionLog     Action = "LOG"
	ActionAlert   Action = "ALERT"
	ActionDisable Action = "DISABLE_CLIENT"
)

type Sample struct {
	ClientID  string
	UpBytes   int64
	DownBytes int64
	At        time.Time
}

func (s Sample) Total() int64 { return s.UpBytes + s.DownBytes }

type Baseline struct {
	ClientID             string
	EWMABytesPerSec      float64
	Variance             float64
	Samples              int
	ConsecutiveAnomalies int
	UpdatedAt            time.Time
}
type Policy struct {
	MinSamples      int
	Alpha           float64
	SigmaMultiplier float64
	MinBytesPerSec  float64
	HardBytesPerSec float64
	Confirmations   int
	Action          Action
}
type Decision struct {
	Suspicious      bool
	Confirmed       bool
	RateBytesPerSec float64
	Threshold       float64
	Reason          string
	Action          Action
}
