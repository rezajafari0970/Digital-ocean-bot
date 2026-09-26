package provisioning

import (
	"time"
)

type ConnectionStage string

const (
	StageTCP       ConnectionStage = "tcp_dial"
	StageHandshake ConnectionStage = "ssh_handshake_auth"
	StageSession   ConnectionStage = "session"
	StageProbe     ConnectionStage = "probe_command"
	StageCommand   ConnectionStage = "command"
)

type StageObservation struct {
	Stage    ConnectionStage
	Duration time.Duration
	Err      error
}

type StageObserver func(StageObservation)
