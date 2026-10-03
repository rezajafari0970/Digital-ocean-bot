package serverguardian

import "testing"

func TestParseSnapshot(t *testing.T) {
	x := parse("xui_active=1\nxui_restarted=0\nmemory_available_mb=512\ndisk_free_mb=2048\nload_1m=0.25\npackage_lock_busy=0\nreboot_required=1\ndb_present=1\ndb_quick_check=ok\n")
	if !x.XUIActive || x.Restarted || x.MemMB != 512 || x.DiskMB != 2048 || x.Load != 0.25 || x.LockBusy || !x.Reboot || !x.DBPresent || x.DBCheck != "ok" {
		t.Fatalf("%+v", x)
	}
}
func TestObserveCommandDoesNotRestart(t *testing.T) {
	c := guardianCommand(false)
	if contains(c, "[ \"0\" = \"1\" ]") == false {
		t.Fatal("observe command missing repair gate")
	}
}
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
