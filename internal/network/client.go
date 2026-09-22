package network

import (
	"net"
	"net/http"
	"net/http/cookiejar"
	"time"
)

type ClientBundle struct {
	AccountID string
	Client    *http.Client
	Jar       http.CookieJar
	Transport *http.Transport
}

func NewIsolatedDirectClient(accountID string) (*ClientBundle, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{
		Proxy:               nil,
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        20,
		IdleConnTimeout:     60 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	client := &http.Client{Transport: tr, Jar: jar, Timeout: 30 * time.Second}
	return &ClientBundle{AccountID: accountID, Client: client, Jar: jar, Transport: tr}, nil
}

func (b *ClientBundle) Validate(accountID string) error {
	if b == nil || b.AccountID != accountID {
		return ErrAccountContextMismatch
	}
	return nil
}

func (b *ClientBundle) CloseIdleConnections() {
	if b != nil && b.Transport != nil {
		b.Transport.CloseIdleConnections()
	}
}
