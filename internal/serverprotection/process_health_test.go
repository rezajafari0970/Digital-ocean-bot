package serverprotection

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestXrayHealthRequiresContinuousFailureWindow(t *testing.T) {
	w := xrayFailureWindow{}
	start := time.Now()
	for n := 0; n <= 12; n++ {
		ready := w.observe(start.Add(time.Duration(n) * 5 * time.Second))
		if ready != (n == 12) {
			t.Fatalf("sample %d ready=%v", n, ready)
		}
	}
	if w.observe(start.Add(90 * time.Second)) {
		t.Fatal("stale observation caused restart")
	}
	if w.observe(start.Add(-time.Second)) {
		t.Fatal("clock regression caused restart")
	}
}
func TestProcessFDAndOwnedXrayListeners(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "123")
	os.MkdirAll(filepath.Join(dir, "fd"), 0700)
	os.MkdirAll(filepath.Join(root, "net"), 0700)
	os.Symlink("/usr/local/x-ui/bin/xray-linux-amd64", filepath.Join(dir, "exe"))
	os.WriteFile(filepath.Join(dir, "limits"), []byte("Max open files            2                    1048576              files\n"), 0600)
	os.Symlink("socket:[42]", filepath.Join(dir, "fd", "1"))
	os.Symlink("socket:[43]", filepath.Join(dir, "fd", "2"))
	os.WriteFile(filepath.Join(root, "net", "tcp"), []byte("0: 00000000:01BB 00000000:0000 0A 0:0 0:0 0 0 0 42\n"), 0600)
	h, err := managedProcesses(root)
	if err != nil || !h.fdKnown || h.fdPercent != 100 {
		t.Fatalf("%+v %v", h, err)
	}
	ok, err := xrayListeners(root, []int{443})
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	os.Remove(filepath.Join(dir, "fd", "1"))
	ok, err = xrayListeners(root, []int{443})
	if err != nil || ok {
		t.Fatalf("foreign socket accepted %v %v", ok, err)
	}
	m := Metrics{MemTotalMB: 1024, MemAvailableMB: 512, DiskFreeMB: 100, InodeFreePercent: 100, ProcessFDKnown: true, ProcessFDPercent: 95}
	e := Engine{}
	now := time.Now()
	e.Evaluate(m, now)
	state, _, blocked := e.Evaluate(m, now.Add(3*time.Second))
	if state != "CRITICAL" || !blocked {
		t.Fatalf("per-process exhaustion missed %s", state)
	}
}
