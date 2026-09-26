package workflow

import "time"

type StepPolicy struct {
	Name        string
	State       State
	MaxAttempts int
	Timeout     time.Duration
}

var DefaultStepPolicies = map[string]StepPolicy{
	"create":        {Name: "create", State: Creating, MaxAttempts: 3, Timeout: 45 * time.Second},
	"wait_resource": {Name: "wait_resource", State: WaitingResource, MaxAttempts: 12, Timeout: 3 * time.Minute},
	"provision":     {Name: "provision", State: Provisioning, MaxAttempts: 64, Timeout: 20 * time.Minute},
	"database":      {Name: "database", State: ImportingDatabase, MaxAttempts: 3, Timeout: 2 * time.Minute},
	"panel":         {Name: "panel", State: ConfiguringPanel, MaxAttempts: 3, Timeout: 2 * time.Minute},
	"clients":       {Name: "clients", State: RegisteringClients, MaxAttempts: 3, Timeout: 2 * time.Minute},
	"traffic":       {Name: "traffic", State: RegisteringTraffic, MaxAttempts: 3, Timeout: 2 * time.Minute},
}
