package adminapi

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var apiReadFault = errors.New("injected database result failure")

type apiReadFaultCase struct {
	name, query string
	handler     func(*Server, http.ResponseWriter, *http.Request)
	row         []driver.Value
}
type apiReadFaultConnector struct {
	tc      apiReadFaultCase
	mode    string
	matched *bool
}

func (c apiReadFaultConnector) Connect(context.Context) (driver.Conn, error) {
	return &apiReadFaultConn{config: c}, nil
}
func (c apiReadFaultConnector) Driver() driver.Driver { return apiReadFaultDriver{} }

type apiReadFaultDriver struct{}

func (apiReadFaultDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("connector required")
}

type apiReadFaultConn struct{ config apiReadFaultConnector }

func (*apiReadFaultConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (*apiReadFaultConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected transaction") }
func (*apiReadFaultConn) Close() error              { return nil }
func (c *apiReadFaultConn) QueryContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(q, c.config.tc.query) {
		*c.config.matched = true
		if c.config.mode == "query" {
			return nil, apiReadFault
		}
		return &apiFaultRows{row: c.config.tc.row, mode: c.config.mode}, nil
	}
	// Counts are fetched only after a complete, empty recent-jobs result.
	if c.config.tc.name == "client_counts" && strings.Contains(q, "FROM client_mutation_jobs ORDER BY") {
		return &apiFaultRows{row: make([]driver.Value, 9)}, nil
	}
	return nil, fmt.Errorf("unexpected query in %s fixture", c.config.tc.name)
}

type apiFaultRows struct {
	row       []driver.Value
	mode      string
	delivered bool
}

func (r *apiFaultRows) Columns() []string {
	n := len(r.row)
	if r.mode == "scan" {
		n++
	}
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprint("column", i)
	}
	return out
}
func (r *apiFaultRows) Close() error {
	if r.mode == "close" {
		return apiReadFault
	}
	return nil
}
func (r *apiFaultRows) Next(out []driver.Value) error {
	switch r.mode {
	case "before":
		return apiReadFault
	case "after", "scan":
		if !r.delivered {
			copy(out, r.row)
			r.delivered = true
			return nil
		}
		if r.mode == "after" {
			return apiReadFault
		}
	}
	return io.EOF
}

func TestAdminListsRejectPartialDatabaseResults(t *testing.T) {
	now := time.Unix(1800000000, 0)
	mark := "ROW-CANARY-MUST-NOT-LEAK"
	cases := []apiReadFaultCase{
		{"configs", "FROM panel_inbound_policies", (*Server).getConfigs, []driver.Value{mark, "sanaei", "http://fixture", "reality", true, int64(1), []byte("[443]"), int64(10000), int64(20000), []byte("[22]"), int64(5), int64(0), int64(3600), int64(0), int64(5), int64(10), "auto", []byte("[]"), int64(1)}},
		{"panels", "FROM panel_instances", (*Server).getConfigPanels, []driver.Value{mark, "sanaei", "http://fixture", true}},
		{"scripts", "FROM install_scripts", (*Server).listInstallScripts, []driver.Value{mark, "fixture", int64(1), "bootstrap", int64(60), int64(3), "digest", true, now}},
		{"installers", "FROM installers", (*Server).listInstallers, []driver.Value{mark, "fixture", int64(1), []byte("{}"), "digest", true, now}},
		{"resources", "FROM resources r", (*Server).accountResources, []driver.Value{mark, "fixture-resource", "server", "active", true, []byte("{}"), now, now, "vultr"}},
		{"audit", "FROM audit_events", (*Server).audit, []driver.Value{int64(1), mark, "fixture", "delete", "server", "fixture-resource", "ok", "fixture", now}},
		{"templates", "FROM xui_database_templates", (*Server).listTemplates, []driver.Value{mark, "fixture", int64(1), "digest", int64(1024), true, now}},
		{"residential", "FROM residential_proxies", (*Server).residentialProxies, []driver.Value{mark, "fixture", "socks5", "127.0.0.1", int64(1080), "", "healthy", "127.0.0.1", "ZZ", int64(10), "fixture-tag", int64(1), true, false, nil, nil, "", float64(1), float64(10), int64(1)}},
		{"proxies", "FROM proxies ORDER BY", (*Server).proxies, []driver.Value{mark, "fixture", "socks5", "127.0.0.1", int64(1080), "healthy", "127.0.0.1", "ZZ", "AS0", int64(10), int64(0), nil, "generic"}},
		{"client_recent", "FROM client_mutation_jobs ORDER BY", (*Server).listClientMutations, []driver.Value{mark, "DELETE", "fixture", "SUCCEEDED", int64(1), "", now, now, nil}},
		{"client_counts", "FROM client_mutation_jobs GROUP BY", (*Server).listClientMutations, []driver.Value{mark, int64(1)}},
	}
	for _, tc := range cases {
		for _, mode := range []string{"query", "before", "after", "scan", "close"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				matched := false
				db := sql.OpenDB(apiReadFaultConnector{tc: tc, mode: mode, matched: &matched})
				defer db.Close()
				request := httptest.NewRequest(http.MethodGet, "/", nil)
				request.SetPathValue("id", "00000000-0000-4000-8000-000000000001")
				response := httptest.NewRecorder()
				tc.handler(&Server{DB: db}, response, request)
				if !matched {
					t.Fatal("fixture did not reach intended database boundary")
				}
				if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), mark) || strings.Contains(response.Body.String(), apiReadFault.Error()) {
					t.Fatalf("partial or diagnostic data escaped: status=%d body=%s", response.Code, response.Body.String())
				}
			})
		}
	}
}
