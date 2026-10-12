package linode

import (
	"context"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"testing"
)

func TestRestrictedAccessCannotImplyAbsenceOrMutate(t *testing.T) {
	for _, body := range []string{`{"restricted":true}`, "{}", "null"} {
		calls := 0
		opened, e := (Factory{}).Open(context.Background(), providers.OpenRequest{AccountID: "fixture-account", Credentials: credential{}, HTTPClient: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.Path != "/v4/profile" || r.Method != "GET" {
				t.Fatal("restricted account made unsafe request", r.Method, r.URL)
			}
			return response(200, body), nil
		})}})
		if e != nil {
			t.Fatal(e)
		}
		d := opened.(*Driver)
		if _, e = d.Inventory(context.Background()); e == nil {
			t.Fatal("partial inventory accepted")
		}
		if _, e = d.GetServer(context.Background(), "7"); e == nil || providers.IsClass(e, providers.ErrorNotFound) {
			t.Fatal("restricted access became absence", e)
		}
		if e = d.DeleteServer(context.Background(), "7"); e == nil {
			t.Fatal("restricted deletion accepted")
		}
		if _, e = d.CreateServer(context.Background(), request(t)); e == nil {
			t.Fatal("restricted create accepted")
		}
		if calls != 4 {
			t.Fatal(calls)
		}
	}
}

func TestProfileNotFoundIsNeverInstanceAbsence(t *testing.T) {
	calls := 0
	opened, e := (Factory{}).Open(context.Background(), providers.OpenRequest{AccountID: "fixture-account", Credentials: credential{}, HTTPClient: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/v4/profile" {
			t.Fatal("unverified access reached instance endpoint")
		}
		return response(404, "{}"), nil
	})}})
	if e != nil {
		t.Fatal(e)
	}
	d := opened.(*Driver)
	if _, e = d.GetServer(context.Background(), "7"); !providers.IsClass(e, providers.ErrorUnavailable) {
		t.Fatal("profile absence became server absence", e)
	}
	if e = d.DeleteServer(context.Background(), "7"); !providers.IsClass(e, providers.ErrorUnavailable) {
		t.Fatal("profile absence became deletion success", e)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}
func TestPublicIPv4ReadinessRejectsSpecialUse(t *testing.T) {
	for _, address := range []string{"0.1.2.3", "10.1.2.3", "100.64.1.1", "100.127.255.254", "127.0.0.1", "169.254.1.1", "172.16.1.1", "192.0.0.8", "192.0.2.1", "192.88.99.1", "192.168.1.1", "198.18.1.1", "198.19.1.1", "198.51.100.1", "203.0.113.8", "224.0.0.1", "240.0.0.1", "255.255.255.255", "::1", "::ffff:8.8.8.8", "malformed"} {
		t.Run(address, func(t *testing.T) {
			x := normalizeServer(serverData{ID: 1, Status: "running", IPv4: []string{address}})
			if x.Ready || x.PrimaryIPv4 != "" {
				t.Fatal("special-use address became ready", x)
			}
		})
	}
	for _, address := range []string{"8.8.8.8", "1.1.1.1", "45.79.0.1", "100.63.255.254", "100.128.0.1"} {
		if got := publicIPv4(address); got != address {
			t.Fatal("public address rejected", address, got)
		}
	}
	x := normalizeServer(serverData{ID: 1, Status: "running", IPv4: []string{"100.64.0.1", "8.8.8.8"}})
	if !x.Ready || x.PrimaryIPv4 != "8.8.8.8" {
		t.Fatal(x)
	}
}
