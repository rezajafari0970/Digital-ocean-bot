package serverprotection

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Replay the actual overflowing TCP budget observed on 158.247.236.58.
// Ordinary free RAM and CPU counters alone must not classify it as ready.
func TestObservedTCPMemoryExhaustion(t *testing.T) {
	m := healthyMetrics()
	if err := json.Unmarshal([]byte(`{"tcp_memory_supported":true,"tcp_memory_used_mb":83.62890625,"tcp_memory_pressure_mb":51.07421875,"tcp_memory_max_mb":76.59375}`), &m); err != nil {
		t.Fatal(err)
	}
	var e Engine
	now := time.Unix(1000, 0)
	e.Evaluate(m, now)
	state, reason, blocked := e.Evaluate(m, now.Add(2*time.Second))
	if state != "CRITICAL" || !blocked || !strings.Contains(reason, "TCP memory") {
		t.Fatalf("TCP exhaustion unguarded: %s %q blocked=%v", state, reason, blocked)
	}
}

func TestTCPBudgetCounters(t *testing.T) {
	for _, tc := range []struct {
		name, sock, limits string
		page               int
		wantUsed           float64
		valid              bool
	}{
		{"observed", "TCP: inuse 856 orphan 102 tw 1067 alloc 14375 mem 21409", "9804 13075 19608", 4096, 83.62890625, true},
		{"zero", "TCP: mem 0 inuse 0", "100 200 300", 4096, 0, true},
		{"large_pages", "TCP: mem 32", "8 16 64", 65536, 2, true},
		{"missing", "UDP: inuse 2 mem 100", "100 200 300", 4096, 0, false},
		{"negative", "TCP: mem -1", "100 200 300", 4096, 0, false},
		{"nan", "TCP: mem NaN", "100 200 300", 4096, 0, false},
		{"overflow", "TCP: mem 18446744073709551616", "100 200 300", 4096, 0, false},
		{"duplicated", "TCP: mem 12 mem 18", "100 200 300", 4096, 0, false},
		{"partial", "TCP: mem", "100 200 300", 4096, 0, false},
		{"bad_order", "TCP: mem 32", "300 200 100", 4096, 0, false},
		{"zero_pressure", "TCP: mem 32", "0 0 100", 4096, 0, false},
		{"missing_limit", "TCP: mem 32", "100 200", 4096, 0, false},
		{"page_size", "TCP: mem 32", "100 200 300", 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			used, pressure, max, ok := tcpMemory([]byte(tc.sock), []byte(tc.limits), tc.page)
			if ok != tc.valid || used != tc.wantUsed {
				t.Fatalf("used=%v pressure=%v max=%v valid=%v", used, pressure, max, ok)
			}
			if ok && (pressure <= 0 || max < pressure) {
				t.Fatal("invalid budget accepted")
			}
		})
	}
}

func TestTCPBudgetRecoveryRequiresKnownLowWatermark(t *testing.T) {
	var e Engine
	m := healthyMetrics()
	m.TCPMemorySupported = true
	m.TCPMemoryMaxMB = 100
	m.TCPMemoryPressureMB = 60
	now := time.Unix(1000, 0)
	for _, tc := range []struct {
		sec     int
		used    float64
		known   bool
		state   string
		blocked bool
	}{
		{0, 95, true, "WARM", false},
		{2, 95, true, "CRITICAL", true},
		{3, 80, true, "CRITICAL", true},
		{40, 80, true, "CRITICAL", true},
		{41, 0, false, "CRITICAL", true},
		{42, 59, true, "RECOVERING", true},
		{71, 59, true, "RECOVERING", true},
		{72, 59, true, "READY", false},
		{80, 70, true, "WARM", false},
		{81, 95, true, "WARM", false},
		{82, 20, true, "READY", false},
	} {
		m.TCPMemoryUsedMB = tc.used
		m.TCPMemorySupported = tc.known
		state, reason, blocked := e.Evaluate(m, now.Add(time.Duration(tc.sec)*time.Second))
		if state != tc.state || blocked != tc.blocked {
			t.Fatalf("at %ds got %s %s blocked=%v", tc.sec, state, reason, blocked)
		}
	}
	// Unsupported kernels do not acquire a new admission block.
	var unsupported Engine
	m = healthyMetrics()
	state, _, blocked := unsupported.Evaluate(m, now)
	if state != "READY" || blocked {
		t.Fatal("unknown optional counters treated as pressure")
	}
}
