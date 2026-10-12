package advancedreality

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func makeLab(t *testing.T) (Lab, Receipt, []byte) {
	t.Helper()
	l := Lab{Dir: filepath.Join(t.TempDir(), "experiment")}
	b := []byte("  " + baseline + "\n")
	r, e := l.Init(b, "26.10.10")
	if e != nil {
		t.Fatal(e)
	}
	return l, r, b
}
func TestLabPersistRollbackRemoveExactBytes(t *testing.T) {
	l, r, b := makeLab(t)
	if r.CurrentSHA256 != Digest(b) {
		t.Fatal("original bytes lost")
	}
	c := []byte(strings.Replace(string(b), `"bufferSize":64`, `"bufferSize":128`, 1))
	x, e := l.Stage(c, r.CurrentSHA256, "stage-0001")
	if e != nil {
		t.Fatal(e)
	}
	if e = l.Remove(x.CurrentSHA256); e != ErrConflict {
		t.Fatal("unrestored lab removable", e)
	}
	// Reopening models a process restart: there is no in-memory source of truth.
	reopened := Lab{Dir: l.Dir}
	got, e := reopened.Status()
	if e != nil || got != x {
		t.Fatal(got, e)
	}
	if _, e = reopened.Rollback(r.CurrentSHA256, "rollback-old"); e != ErrConflict {
		t.Fatal("stale CAS accepted", e)
	}
	z, e := reopened.Rollback(x.CurrentSHA256, "rollback-0001")
	if e != nil || z.CurrentSHA256 != Digest(b) {
		t.Fatal(z, e)
	}
	old, e := reopened.Stage(c, r.CurrentSHA256, "stage-0001")
	if e != nil || old != x {
		t.Fatal("idempotent receipt lost", e)
	}
	now, e := reopened.Status()
	if e != nil || now != z {
		t.Fatal("replay changed state")
	}
	if _, e = reopened.Stage(b, z.CurrentSHA256, "stage-0001"); e != ErrConflict {
		t.Fatal("key reuse allowed")
	}
	if e = reopened.Remove(z.CurrentSHA256); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(l.Dir); !os.IsNotExist(e) {
		t.Fatal("not removed")
	}
}
func TestLabConcurrentCAS(t *testing.T) {
	l, r, b := makeLab(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			n := []string{"80", "96", "112", "128", "144", "160", "176", "192"}[i]
			c := []byte(strings.Replace(string(b), `"bufferSize":64`, `"bufferSize":`+n, 1))
			_, e := l.Stage(c, r.CurrentSHA256, "request-"+n)
			mu.Lock()
			defer mu.Unlock()
			if e == nil {
				success++
			} else if e != ErrConflict {
				t.Error(e)
			}
		}(i)
	}
	wg.Wait()
	if success != 1 {
		t.Fatal("CAS winners", success)
	}
}
func TestLabProtectsForeignFilesAndSymlinks(t *testing.T) {
	l, r, _ := makeLab(t)
	foreign := filepath.Join(l.Dir, "foreign")
	if e := os.WriteFile(foreign, []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := l.Remove(r.CurrentSHA256); e != ErrStorage {
		t.Fatal(e)
	}
	if b, e := os.ReadFile(foreign); e != nil || string(b) != "keep" {
		t.Fatal("foreign file lost")
	}
	link := filepath.Join(t.TempDir(), "link")
	if e := os.Symlink(l.Dir, link); e != nil {
		t.Fatal(e)
	}
	if _, e := (Lab{Dir: link}).Status(); e != ErrStorage {
		t.Fatal("symlink allowed")
	}
	if _, e := l.Init([]byte(baseline), "26.10.10"); e != ErrStorage {
		t.Fatal("existing directory reused")
	}
}
func TestLabPermissionsAndInterruptedPendingWrite(t *testing.T) {
	l, r, _ := makeLab(t)
	f := filepath.Join(l.Dir, "state.json")
	info, _ := os.Stat(f)
	if info.Mode().Perm() != 0600 {
		t.Fatal("permissions")
	}
	// An interrupted pre-rename write does not replace committed state. Its
	// unknown residue blocks uninstall rather than being recursively deleted.
	if e := os.WriteFile(filepath.Join(l.Dir, "state.pending-interrupted"), []byte("incomplete"), 0600); e != nil {
		t.Fatal(e)
	}
	got, e := l.Status()
	if e != nil || got != r {
		t.Fatal(got, e)
	}
	if e = l.Remove(r.CurrentSHA256); e != ErrStorage {
		t.Fatal("removed unknown residue")
	}
	if e = os.Chmod(l.Dir, 0755); e != nil {
		t.Fatal(e)
	}
	if _, e = l.Status(); e != ErrStorage {
		t.Fatal("public directory accepted")
	}
}
