package provisioning

import "time"

type State string

const (
	Pending           State = "PENDING"
	WaitingSSH        State = "WAITING_SSH"
	Bootstrapping     State = "BOOTSTRAPPING"
	InstallingPanel   State = "INSTALLING_PANEL"
	ImportingDatabase State = "IMPORTING_DATABASE"
	Verifying         State = "VERIFYING"
	Completed         State = "COMPLETED"
	Failed            State = "FAILED"
)

type Target struct {
	AccountID    string
	DropletID    string
	Host         string
	Port         int
	User         string
	KeySecretRef string
}
type Step struct {
	Name        string
	State       State
	Attempt     int
	LastError   string
	StartedAt   *time.Time
	CompletedAt *time.Time
}
type Run struct {
	ID          string
	AccountID   string
	DropletID   string
	State       State
	CurrentStep string
	Attempt     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
