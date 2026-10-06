package serverprotection

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func healthyMetrics() Metrics {
	return Metrics{MemTotalMB: 1024, MemAvailableMB: 512, DiskFreeMB: 4096, InodeFreePercent: 50}
}
func TestPressureHysteresisAndFaultRecovery(t *testing.T) {
	e := Engine{}
	m := healthyMetrics()
	now := time.Unix(1000, 0)
	for _, x := range []struct {
		at       time.Duration
		pressure bool
		state    string
		blocked  bool
	}{{0, true, "WARM", false}, {time.Second, true, "WARM", false}, {2 * time.Second, true, "CRITICAL", true}, {3 * time.Second, false, "RECOVERING", true}, {32 * time.Second, false, "RECOVERING", true}, {33 * time.Second, false, "READY", false}} {
		m.FDPercent = 0
		if x.pressure {
			m.FDPercent = 95
		}
		state, _, block := e.Evaluate(m, now.Add(x.at))
		if state != x.state || block != x.blocked {
			t.Fatalf("%+v got %s %v", x, state, block)
		}
	}
	m.MemAvailableMB = 8
	s, _, b := e.Evaluate(m, now.Add(time.Minute))
	if s != "CRITICAL" || !b {
		t.Fatal("emergency not immediate")
	}
	m = healthyMetrics()
	e.Evaluate(m, now.Add(61*time.Second))
	m.MemAvailableMB = 8
	e.Evaluate(m, now.Add(70*time.Second))
	m = healthyMetrics()
	s, _, b = e.Evaluate(m, now.Add(91*time.Second))
	if s != "RECOVERING" || !b {
		t.Fatal("pressure relapse incorrectly cleared")
	}
}
func TestProtectionReceiptRejectsFaults(t *testing.T) {
	p := Policy{Revision: 3, Enabled: true}
	good := Status{Version: Version, Revision: 3, Enabled: true, AgentRunning: true, NFTSupported: true, State: "READY", Ports: []int{443}}
	if e := good.ValidateReceipt(p); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*Status){func(s *Status) { s.Revision-- }, func(s *Status) { s.State = "ERROR" }, func(s *Status) { s.SampleAgeMS = 6000 }, func(s *Status) { s.AgentRunning = false }, func(s *Status) { s.AdmissionBlocked = true }, func(s *Status) { s.State = "CRITICAL" }, func(s *Status) { s.Ports = nil }} {
		s := good
		mutate(&s)
		if s.ValidateReceipt(p) == nil {
			t.Fatal("fault receipt accepted", s)
		}
	}
	p.Enabled = false
	off := Status{Version: Version, Revision: 3, State: "DISABLED"}
	if off.ValidateReceipt(p) != nil {
		t.Fatal("off rejected")
	}
	off.AgentRunning = true
	if off.ValidateReceipt(p) == nil {
		t.Fatal("running disable accepted")
	}
}
func TestParsePortsProtectsManagementAndLocalServices(t *testing.T) {
	raw := `{"inbounds":[{"protocol":"vless","port":443},{"protocol":"vless","port":"8443","streamSettings":{"network":"raw"}},{"protocol":"vless","port":22},{"protocol":"vless","port":2053},{"protocol":"vless","port":1234,"listen":"127.0.0.1"},{"protocol":"socks","port":1080},{"protocol":"vless","port":8888,"streamSettings":{"network":"ws"}}]}`
	got, e := ParsePorts(strings.NewReader(raw), []int{2053})
	if e != nil || len(got) != 2 || got[0] != 443 || got[1] != 8443 {
		t.Fatal(got, e)
	}
	if _, e = ParsePorts(strings.NewReader("{"), nil); e == nil {
		t.Fatal("malformed accepted")
	}
}

func TestLegacyNFTOwnerProof(t *testing.T) {
	good := "table inet dob_guardian { # handle 8\n\tcomment " + strconv.Quote(nftOwner) + "\n}\n"
	if !legacyTableOwner([]byte(good), 8) {
		t.Fatal("legacy ownership proof rejected")
	}
	if legacyTableOwner([]byte(good), 9) || legacyTableOwner([]byte(strings.Replace(good, nftOwner, "foreign", 1)), 8) || legacyTableOwner([]byte("table inet dob_guardian { # handle 8\nchain x {\n comment "+strconv.Quote(nftOwner)+"\n}"), 8) {
		t.Fatal("foreign or replaced table accepted")
	}
}
