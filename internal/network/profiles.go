package network

import (
	"crypto/tls"
	"net/http"
	"net/http/cookiejar"
)

type TLSProfile struct {
	AccountID    string
	Config       *tls.Config
	SessionCache tls.ClientSessionCache
}

type HTTPProfile struct {
	AccountID string
	Client    *http.Client
	Jar       http.CookieJar
	Transport *http.Transport
}

type BrowserProfile struct {
	AccountID        string
	ProfileDir       string
	StorageNamespace string
	CacheNamespace   string
	AuthNamespace    string
}

func NewTLSProfile(accountID string) *TLSProfile {
	cache := tls.NewLRUClientSessionCache(64)
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ClientSessionCache: cache}
	return &TLSProfile{AccountID: accountID, Config: cfg, SessionCache: cache}
}

func NewHTTPProfile(accountID string) (*HTTPProfile, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{TLSClientConfig: NewTLSProfile(accountID).Config, ForceAttemptHTTP2: true}
	return &HTTPProfile{AccountID: accountID, Client: &http.Client{Transport: PrivacyTransport{Base: tr}, Jar: jar}, Jar: jar, Transport: tr}, nil
}

func NewBrowserProfile(accountID, root string) BrowserProfile {
	ns := AccountNamespace(accountID)
	return BrowserProfile{AccountID: accountID, ProfileDir: root + "/" + ns, StorageNamespace: "browser:" + ns + ":storage", CacheNamespace: "browser:" + ns + ":cache", AuthNamespace: "browser:" + ns + ":auth"}
}
