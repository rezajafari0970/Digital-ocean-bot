package observability

import (
	"bytes"
	"strings"
	"testing"
)

func TestMetricsOutput(t *testing.T) {
	m := NewMetrics()
	m.Inc("bot_operations_total")
	m.Inc("bot_operations_total")
	m.Set("bot_workers", 3)
	var b bytes.Buffer
	m.WritePrometheus(&b)
	out := b.String()
	if !strings.Contains(out, "bot_operations_total 2") || !strings.Contains(out, "bot_workers 3") {
		t.Fatal(out)
	}
}
