package reality

import "context"

type Prober interface {
	Probe(context.Context, Candidate, int) (Observation, error)
}
type RunFunc func(context.Context, string) (string, error)
type SSHProber struct{ Run RunFunc }
