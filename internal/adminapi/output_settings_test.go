package adminapi

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestOutputSettingsStrictRevisionAndNoNativeMutation(t *testing.T) {
	db := adminTestDB(t)
	s := &Server{DB: db}
	read := func() outputSettings {
		t.Helper()
		w := httptest.NewRecorder()
		s.getOutputSettings(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var p outputSettings
		if e := json.Unmarshal(w.Body.Bytes(), &p); e != nil {
			t.Fatal(e)
		}
		return p
	}
	put := func(body string) int {
		w := httptest.NewRecorder()
		s.putOutputSettings(w, httptest.NewRequest("PUT", "/", strings.NewReader(body)))
		return w.Code
	}
	before := read()
	if before.Revision != 1 || before.IncludeXHTTPExtra || before.IncludeFinalMask {
		t.Fatal(before)
	}
	var nativeBefore string
	if err := db.QueryRow("SELECT config::text FROM reality_transport_policy").Scan(&nativeBefore); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		"{}", `{"revision":1,"include_xhttp_extra":true}`, `{"revision":1,"include_xhttp_extra":null,"include_finalmask":false}`,
		`{"revision":null,"include_xhttp_extra":false,"include_finalmask":false}`, `{"revision":1,"include_xhttp_extra":false,"include_finalmask":null}`,
		`{"revision":0,"include_xhttp_extra":false,"include_finalmask":false}`, `{"revision":1,"include_xhttp_extra":"false","include_finalmask":false}`,
		`{"revision":1,"include_xhttp_extra":false,"include_finalmask":false,"apply_existing":true}`,
		`{"revision":1,"include_xhttp_extra":false,"include_finalmask":false}{}`,
	} {
		if got := put(body); got != 400 {
			t.Fatal(body, got)
		}
	}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- put(`{"revision":1,"include_xhttp_extra":true,"include_finalmask":true}`)
		}()
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for c := range codes {
		counts[c]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatal(counts)
	}
	if p := read(); p.Revision != 2 || !p.IncludeFinalMask || !p.IncludeXHTTPExtra {
		t.Fatal(p)
	}
	var nativeAfter string
	var jobs int
	if err := db.QueryRow("SELECT config::text FROM reality_transport_policy").Scan(&nativeAfter); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT count(*) FROM reality_transport_apply_jobs").Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if nativeBefore != nativeAfter || jobs != 0 {
		t.Fatal("output policy changed native intent")
	}
}
func TestOutputOptionsApplyToExistingSnapshotsAndEveryRoute(t *testing.T) {
	db := adminTestDB(t)
	f := seedAccountOutput(t, db)
	s := &Server{DB: db}
	// Add fields to already-existing snapshots, including legacy browser metadata.
	sqlMust(t, db, `UPDATE output_config_snapshots SET uri=split_part(uri,'#',1)||CASE WHEN position('?' in split_part(uri,'#',1))>0 THEN '&' ELSE '?' END||'extra=%7B%22xPaddingBytes%22%3A%22100-1000%22%7D&fm=%7B%22tcp%22%3A%5B%5D%7D&browserDialerMode=WebView#'||split_part(uri,'#',2)`)
	var original string
	if err := db.QueryRow("SELECT md5(string_agg(uri,',' ORDER BY uri)) FROM output_config_snapshots").Scan(&original); err != nil {
		t.Fatal(err)
	}
	shares := map[string]string{}
	accounts := map[string]string{}
	for _, class := range []string{"ALL", "DIRECT", "RESIDENTIAL"} {
		token := "output-options-" + class
		shares[class] = token
		sqlMust(t, db, "INSERT INTO output_share_tokens(token,route_class) VALUES($1,$2)", token, class)
		if class != "ALL" {
			accounts[class] = accountOutputToken(t, s, f[0].Account, class)
		}
	}
	for _, extra := range []bool{false, true} {
		for _, mask := range []bool{false, true} {
			sqlMust(t, db, "UPDATE output_export_policy SET include_xhttp_extra=$1,include_finalmask=$2,revision=revision+1", extra, mask)
			sqlMust(t, db, "UPDATE output_config_snapshots SET last_seen_at=now()")
			check := func(w *httptest.ResponseRecorder, min int) {
				t.Helper()
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
				lines := strings.Fields(w.Body.String())
				if len(lines) < min {
					t.Fatal("missing output", len(lines))
				}
				for _, line := range lines {
					u, e := url.Parse(line)
					if e != nil {
						t.Fatal(e)
					}
					if u.Query().Has("extra") != extra || u.Query().Has("fm") != mask || u.Query().Has("browserDialerMode") {
						t.Fatal("incorrect output options", extra, mask, line)
					}
					if u.Scheme != "vless" || u.User == nil || u.Port() != "443" {
						t.Fatal("required identity changed")
					}
				}
			}
			for _, class := range []string{"ALL", "DIRECT", "RESIDENTIAL"} {
				w := httptest.NewRecorder()
				s.outputConfigs(w, httptest.NewRequest("GET", "/?route_class="+class, nil))
				check(w, 4)
				w = httptest.NewRecorder()
				r := httptest.NewRequest("GET", "/", nil)
				r.SetPathValue("token", shares[class])
				s.sharedOutput(w, r)
				check(w, 4)
				if class != "ALL" {
					w = readAccountOutput(s, accounts[class], "")
					check(w, 1)
					if len(strings.Fields(w.Body.String())) != 1 {
						t.Fatal("account scope escaped")
					}
				}
			}
		}
	}
	var after string
	if err := db.QueryRow("SELECT md5(string_agg(uri,',' ORDER BY uri)) FROM output_config_snapshots").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != original {
		t.Fatal("stored snapshots were rewritten")
	}
	// A missing policy is an error even with no publishable rows.
	sqlMust(t, db, "DELETE FROM output_export_policy")
	for _, empty := range []bool{false, true} {
		if empty {
			sqlMust(t, db, "DELETE FROM output_config_snapshots")
		}
		for _, class := range []string{"ALL", "DIRECT", "RESIDENTIAL"} {
			w := httptest.NewRecorder()
			s.outputSnapshotClass(w, httptest.NewRequest("GET", "/", nil), class)
			if w.Code != 500 || strings.Contains(w.Body.String(), "vless://") {
				t.Fatal(fmt.Sprint(empty, class, w.Code))
			}
		}
	}
}
