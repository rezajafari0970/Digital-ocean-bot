package network

import "time"

type ProxyType string
type ProxyStatus string
type RouteMode string

const (
	ProxyHTTP          ProxyType   = "http"
	ProxyHTTPS         ProxyType   = "https"
	ProxySOCKS5        ProxyType   = "socks5"
	RouteDirect        RouteMode   = "direct"
	RouteProxyRequired RouteMode   = "proxy_required"
	StatusUnknown      ProxyStatus = "unknown"
	StatusHealthy      ProxyStatus = "healthy"
	StatusDegraded     ProxyStatus = "degraded"
	StatusDown         ProxyStatus = "down"
)

type Proxy struct {
	ID            string
	Name          string
	Type          ProxyType
	Host          string
	Port          int
	Username      string
	SecretRef     string
	Status        ProxyStatus
	ExitIP        string
	Country       string
	ASN           string
	LastCheckedAt *time.Time
	LastSuccessAt *time.Time
	FailureCount  int
}

type Profile struct {
	ID        string
	AccountID string
	Mode      RouteMode
	ProxyID   *string
}
