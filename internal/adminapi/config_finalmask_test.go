package adminapi

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configplane"
)

func TestConfigProfileFinalMaskPersistence(t *testing.T) {
	db := adminTestDB(t)
	s := &Server{DB: db}
	mask := `{"tcp":[{"type":"fragment","settings":{"packets":"tlshello","maxSplit":"0","length":"1-104","delay":"0"}},{"type":"fragment","settings":{"packets":"1-1","maxSplit":"11","length":"1-114","delay":"1"}}],"udp":[{"type":"header-custom","settings":{"client":[{"rand":16,"randRange":"0-255"}],"server":[{"rand":16,"randRange":"0-255"}]}}]}`
	// Two independently valid mask fields must fit in one profile request.
	largeMask := strings.Replace(mask, `"udp":`, strings.Repeat(" ", 17000)+`"udp":`, 1)
	for _, security := range []string{"none", "tls", "reality"} {
		p := configplane.Profile{Name: "FinalMask " + security, Fleet: true, Classes: []string{"DIRECT", "RESIDENTIAL"}, Count: 1, Spec: configdriver.Spec{Protocol: "vless", Network: "tcp", Security: security, Port: 19443}, ClassSettings: map[string]configdriver.ClassSettings{"DIRECT": {Count: 1, Publish: true, FinalMask: largeMask, ExportFinalMask: true}, "RESIDENTIAL": {Count: 1, Publish: true, FinalMask: largeMask, ExportFinalMask: false}}}
		if security == "reality" {
			p.Spec.SNI = "example.com"
			d := p.ClassSettings["DIRECT"]
			d.Flow = "xtls-rprx-vision"
			p.ClassSettings["DIRECT"] = d
		}
		code, saved := profileSave(t, s, p, "POST")
		if code != 200 {
			t.Fatal("mask rejected", security, code)
		}
		reloaded, e := configplane.Read(context.Background(), db, saved.ID)
		if e != nil || !reflect.DeepEqual(reloaded.ClassSettings, p.ClassSettings) {
			t.Fatal("mask/policy not retained", security, e)
		}
		d := saved.ClassSettings["DIRECT"]
		d.FinalMask = strings.Replace(mask, `"delay":"0"`, `"delay":null`, 1)
		saved.ClassSettings["DIRECT"] = d
		if code, _ = profileSave(t, s, saved, "PUT"); code != 400 {
			t.Fatal("invalid mask saved")
		}
		unchanged, e := configplane.Read(context.Background(), db, saved.ID)
		if e != nil || unchanged.Revision != reloaded.Revision || !reflect.DeepEqual(unchanged.ClassSettings, reloaded.ClassSettings) {
			t.Fatal("rejected save changed profile")
		}
		d = unchanged.ClassSettings["DIRECT"]
		d.ExportFinalMask = false
		unchanged.ClassSettings["DIRECT"] = d
		code, off := profileSave(t, s, unchanged, "PUT")
		if code != 200 || off.ClassSettings["DIRECT"].FinalMask != largeMask || off.ClassSettings["DIRECT"].ExportFinalMask {
			t.Fatal("export toggle lost saved mask")
		}
	}
}
