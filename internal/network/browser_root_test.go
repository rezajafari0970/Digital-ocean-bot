package network

import "testing"

func TestBrowserSessionRootIsProcessScoped(t *testing.T) {
	t.Setenv("DOB_BROWSER_SESSIONS_DIR", "")
	root, err := BrowserSessionRoot()
	if err != nil || root != "/var/lib/digital-ocean-bot/browser-sessions" {
		t.Fatal(root, err)
	}
	private := t.TempDir()
	t.Setenv("DOB_BROWSER_SESSIONS_DIR", private)
	root, err = BrowserSessionRoot()
	if err != nil || root != private {
		t.Fatal(root, err)
	}
	for _, bad := range []string{"/", "relative", "/tmp/../", "/tmp//sessions"} {
		t.Setenv("DOB_BROWSER_SESSIONS_DIR", bad)
		if _, err = BrowserSessionRoot(); err == nil {
			t.Fatal("unsafe root accepted", bad)
		}
	}
}
