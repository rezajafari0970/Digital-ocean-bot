package observability

import (
	"fmt"
	"io"
	"sort"
	"sync"
)

type Metrics struct {
	mu       sync.RWMutex
	counters map[string]uint64
	gauges   map[string]float64
}

func NewMetrics() *Metrics {
	return &Metrics{counters: map[string]uint64{}, gauges: map[string]float64{}}
}
func (m *Metrics) Inc(name string)            { m.mu.Lock(); defer m.mu.Unlock(); m.counters[name]++ }
func (m *Metrics) Set(name string, v float64) { m.mu.Lock(); defer m.mu.Unlock(); m.gauges[name] = v }
func (m *Metrics) WritePrometheus(w io.Writer) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]string, 0, len(m.counters)+len(m.gauges))
	for k := range m.counters {
		keys = append(keys, k)
	}
	for k := range m.gauges {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v, ok := m.counters[k]; ok {
			fmt.Fprintf(w, "%s %d\n", k, v)
		} else {
			fmt.Fprintf(w, "%s %g\n", k, m.gauges[k])
		}
	}
}
