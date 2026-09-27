package reality

import "time"

type Candidate struct {
	Target     string
	ServerName string
	Port       int
}
type Observation struct {
	Candidate  Candidate
	Reachable  bool
	TLSVersion string
	CertValid  bool
	HTTP2      bool
	Samples    int
	Successes  int
	LatencyMS  []int64
	ObservedAt time.Time
}
type Score struct {
	Candidate       Candidate
	Eligible        bool
	MedianLatencyMS int64
	SuccessRatio    float64
	Value           float64
	Reason          string
}

type ScanSelection struct {
	PanelID        string
	ServerNames    []string
	TLSVersion     string
	ALPN           string
	CurveID        string
	CertValid      bool
	CertChainValid bool
	LatencyMS      int64
	ScannedAt      time.Time
}
