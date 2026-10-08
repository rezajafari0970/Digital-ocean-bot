package network

import (
	"crypto/sha256"
	"crypto/tls"
	"sync"
	"time"
)

// Only TLS tickets persist. Every identity gateway, resolver and TCP connection
// is new, so resumption cannot stand in for a fresh exit-IP observation.
type identityTLSCaches struct {
	mu      sync.Mutex
	entries map[[32]byte]*expiringTLSCache
	next    uint64
}
type expiringTLSCache struct {
	cache   tls.ClientSessionCache
	expires time.Time
	used    uint64
}

var probeTLSCaches = identityTLSCaches{entries: make(map[[32]byte]*expiringTLSCache)}

func (c *expiringTLSCache) Get(key string) (*tls.ClientSessionState, bool) {
	if !time.Now().Before(c.expires) {
		return nil, false
	}
	return c.cache.Get(key)
}
func (c *expiringTLSCache) Put(key string, state *tls.ClientSessionState) {
	if time.Now().Before(c.expires) {
		c.cache.Put(key, state)
	}
}

func (s *identityTLSCaches) forRoute(account string, p Proxy, creds ProxyCredentials) *expiringTLSCache {
	key := sha256.Sum256([]byte(account + "\x00" + providerRouteKey(p, creds, "identity")))
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, c := range s.entries {
		if !now.Before(c.expires) {
			delete(s.entries, k)
		}
	}
	c := s.entries[key]
	if c == nil {
		if len(s.entries) >= 128 {
			var oldest [32]byte
			var used uint64
			first := true
			for k, e := range s.entries {
				if first || e.used < used {
					oldest = k
					used = e.used
					first = false
				}
			}
			delete(s.entries, oldest)
		}
		c = &expiringTLSCache{cache: tls.NewLRUClientSessionCache(8), expires: now.Add(10 * time.Minute)}
		s.entries[key] = c
	}
	s.next++
	c.used = s.next
	return c
}

// NewIdentityProxyGateway never shares transports or sockets, even when TLS
// tickets are enabled. TLS defaults and certificate/hostname checks are intact.
func NewIdentityProxyGateway(account string, p Proxy, creds ProxyCredentials, resume bool) (*Gateway, error) {
	g, err := NewAccountProxyGateway(account, p, creds, "identity")
	if err != nil || !resume {
		return g, err
	}
	cache := probeTLSCaches.forRoute(account, p, creds)
	g.Transport.TLSClientConfig = &tls.Config{ClientSessionCache: cache}
	g.resolver.tlsConfig = &tls.Config{ClientSessionCache: cache}
	return g, nil
}
