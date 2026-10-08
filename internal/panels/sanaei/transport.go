package sanaei

import (
	"context"
	"net"
	"net/http"
	"time"
)

// Panel management goes directly to its server and never inherits process proxy
// variables. Cookies and credentials remain in each APIClient, not the transport.
var directPanelTransport = &http.Transport{
	Proxy: nil, ForceAttemptHTTP2: true, MaxIdleConns: 128, MaxIdleConnsPerHost: 4,
	IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second,
	DialContext: func(ctx context.Context, _ string, address string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, "tcp4", address)
	},
}
