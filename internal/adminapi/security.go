package adminapi

import (
	"net/http"
	"sync"
	"time"
)

type loginBucket struct {
	Count        int
	Reset        time.Time
	BlockedUntil time.Time
}
type LoginLimiter struct {
	mu      sync.Mutex
	buckets map[string]loginBucket
	Limit   int
	Window  time.Duration
	Block   time.Duration
}

func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{buckets: map[string]loginBucket{}, Limit: 8, Window: 5 * time.Minute, Block: 15 * time.Minute}
}
func (l *LoginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b := l.buckets[key]
	if now.Before(b.BlockedUntil) {
		return false
	}
	if b.Reset.IsZero() || now.After(b.Reset) {
		b = loginBucket{Reset: now.Add(l.Window)}
	}
	b.Count++
	if b.Count > l.Limit {
		b.BlockedUntil = now.Add(l.Block)
		l.buckets[key] = b
		return false
	}
	l.buckets[key] = b
	return true
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func Secure(next http.Handler) http.Handler { return securityHeaders(next) }
