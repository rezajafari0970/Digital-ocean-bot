package network

import (
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"sync"
	"time"
)

// ProviderTransportPool shares only a raw transport for one exact account route.
// Per-request admission/epoch/identity wrappers remain outside the pool.
type ProviderTransportPool struct {
	mu      sync.Mutex
	entries map[string]pooledGateway
	max     int
}
type pooledGateway struct {
	key           string
	gateway       *Gateway
	created, used time.Time
}

func NewProviderTransportPool(max int) *ProviderTransportPool {
	if max < 1 {
		max = 128
	}
	return &ProviderTransportPool{entries: make(map[string]pooledGateway), max: max}
}
func providerRouteKey(p Proxy, creds ProxyCredentials, revision string) string {
	// Secrets are never used as map keys or logs in plaintext.
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%s\x00%s\x00%s", p.ID, p.Type, p.Host, p.Port, creds.Username, creds.Password, revision))))
}
func (p *ProviderTransportPool) Open(owner string, proxy Proxy, creds ProxyCredentials, revision string) (*Gateway, error) {
	if proxy.Status != StatusHealthy {
		return nil, ErrProxyConfigInvalid
	}
	if p == nil {
		return NewAccountProxyGateway(owner, proxy, creds, "provider_api")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	key := providerRouteKey(proxy, creds, revision)
	entry, ok := p.entries[owner]
	if ok && (entry.key != key || now.Sub(entry.created) >= 10*time.Minute) {
		entry.gateway.Transport.CloseIdleConnections()
		delete(p.entries, owner)
		ok = false
	}
	if !ok {
		if len(p.entries) >= p.max {
			var oldest string
			var at time.Time
			for id, e := range p.entries {
				if oldest == "" || e.used.Before(at) {
					oldest = id
					at = e.used
				}
			}
			p.entries[oldest].gateway.Transport.CloseIdleConnections()
			delete(p.entries, oldest)
		}
		g, err := NewAccountProxyGateway(owner, proxy, creds, "provider_api")
		if err != nil {
			return nil, err
		}
		// Each exact pool entry owns fresh TLS ticket state. Expiry is absolute,
		// matching the transport lifetime, and never extends on resumed use.
		cache := &expiringTLSCache{cache: tls.NewLRUClientSessionCache(8), expires: now.Add(10 * time.Minute)}
		g.Transport.TLSClientConfig = &tls.Config{ClientSessionCache: cache}
		g.resolver.tlsConfig = &tls.Config{ClientSessionCache: cache}
		g.Transport.IdleConnTimeout = 5 * time.Minute
		g.Transport.MaxIdleConnsPerHost = 4
		entry = pooledGateway{key: key, gateway: g, created: now}
	}
	entry.used = now
	p.entries[owner] = entry
	// Each caller gets its own http.Client so adding a guard/observer cannot
	// mutate another runtime's wrapper or stale generation.
	g := *entry.gateway
	client := *g.Client
	g.Client = &client
	g.pooled = true
	return &g, nil
}
func (p *ProviderTransportPool) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, e := range p.entries {
		e.gateway.Transport.CloseIdleConnections()
		delete(p.entries, id)
	}
}
