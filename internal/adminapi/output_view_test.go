package adminapi

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestOutputBrowserViewScriptIsServed(t *testing.T) {
	t.Chdir("../..")
	view := httptest.NewRecorder()
	outputBrowserView(view)
	match := regexp.MustCompile(`<script src="([^"]+)"`).FindStringSubmatch(view.Body.String())
	if len(match) != 2 {
		t.Fatal("missing output script")
	}
	s := Server{WebPath: "/admin"}
	asset := httptest.NewRecorder()
	s.Routes().ServeHTTP(asset, httptest.NewRequest("GET", match[1], nil))
	if asset.Code != 200 || !strings.Contains(asset.Body.String(), "fetch(") {
		t.Fatalf("output script unavailable: %d", asset.Code)
	}
}
