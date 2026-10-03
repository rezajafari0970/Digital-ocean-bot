package usercapacity

import (
	"fmt"
	"sync"
	"time"
)

type bulkRateState struct {
	last   time.Time
	tokens float64
}

type bulkRateLimiter struct {
	mu    sync.Mutex
	state map[string]bulkRateState
}

func (l *bulkRateLimiter) allowance(key string, rate, deficit int, now time.Time) int {
	if rate <= 0 || deficit <= 0 {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state == nil {
		l.state = map[string]bulkRateState{}
	}
	st, ok := l.state[key]
	if !ok {
		st = bulkRateState{last: now, tokens: float64(rate)}
	} else {
		elapsed := now.Sub(st.last).Seconds()
		if elapsed < 0 {
			elapsed = 0
		}
		st.tokens += elapsed * float64(rate)
		if st.tokens > float64(rate) {
			st.tokens = float64(rate)
		}
		st.last = now
	}
	n := int(st.tokens)
	if n > deficit {
		n = deficit
	}
	st.tokens -= float64(n)
	l.state[key] = st
	return n
}

func bulkRateKey(panelID string, inboundID int64) string {
	return fmt.Sprintf("%s/%d", panelID, inboundID)
}

var userCreationLimiter bulkRateLimiter
