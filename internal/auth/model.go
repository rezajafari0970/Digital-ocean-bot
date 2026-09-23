package auth

import "time"

type Role string

const (
	Admin    Role = "admin"
	Operator Role = "operator"
	Viewer   Role = "viewer"
)

type User struct {
	ID           string
	Username     string
	PasswordHash string
	Role         Role
	Enabled      bool
}
type Session struct {
	ID         string
	UserID     string
	TokenHash  []byte
	ExpiresAt  time.Time
	CreatedAt  time.Time
	LastSeenAt time.Time
}
type Principal struct {
	UserID    string
	Username  string
	Role      Role
	SessionID string
}

func (p Principal) CanWrite() bool { return p.Role == Admin || p.Role == Operator }
func (p Principal) CanAdmin() bool { return p.Role == Admin }
