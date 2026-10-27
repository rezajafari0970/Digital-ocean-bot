package kamatera

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"testing"
)

type absenceReceipt struct {
	confirms, presents int
	err                error
}

func (a *absenceReceipt) Confirm(context.Context, string) error { a.confirms++; return a.err }
func (a *absenceReceipt) Present(context.Context, string) error { a.presents++; return nil }

func TestKamateraFailedExactReadNeedsCompleteCorroboration(t *testing.T) {
	other := `[{"id":"22222222-2222-4222-8222-222222222222","name":"other","datacenter":"EU","power":"on"}]`
	target := `[{"id":"` + serverID + `","name":"target","datacenter":"EU","power":"on"}]`
	for _, tc := range []struct {
		name, basic, detailed     string
		basicStatus, detailStatus int
		proof, present, absent    bool
	}{
		{"valid matching lists", other, other, 200, 200, true, false, true},
		{"valid empty lists", "[]", "[]", 200, 200, true, false, true},
		{"confirmed empty wildcard", "[]", emptyWildcardResponse, 200, 500, true, false, true},
		{"basic still present", target, "[]", 200, 200, false, true, false},
		{"detailed still present", "[]", target, 200, 200, false, true, false},
		{"different inventories", other, "[]", 200, 200, false, false, false},
		{"basic null", "null", "[]", 200, 200, false, false, false},
		{"basic partial", "[]", "[]", 206, 200, false, false, false},
		{"basic auth", "{}", "[]", 401, 200, false, false, false},
		{"detail partial", "[]", "[]", 200, 206, false, false, false},
		{"detail null", "[]", "null", 200, 200, false, false, false},
		{"detail malformed", "[]", "[", 200, 200, false, false, false},
		{"detail unavailable", "[]", `{"message":"General Error. (1)"}`, 200, 500, false, false, false},
		{"detail auth", "[]", "{}", 200, 403, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exact, full, basic := 0, 0, 0
			d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
				if r.Method == "GET" && r.URL.Path == "/service/servers" {
					basic++
					x := reply(tc.basic)
					x.StatusCode = tc.basicStatus
					return x, nil
				}
				if r.Method != "POST" || r.URL.Path != "/service/server/info" {
					t.Fatal("unexpected mutation", r.Method, r.URL.Path)
				}
				var selector map[string]string
				if err := json.NewDecoder(r.Body).Decode(&selector); err != nil {
					t.Fatal(err)
				}
				if selector["id"] == serverID {
					exact++
					x := reply(`{"message":"General Error. (1)"}`)
					x.StatusCode = 500
					return x, nil
				}
				if selector["name"] != ".*" {
					t.Fatal("scope changed")
				}
				full++
				x := reply(tc.detailed)
				x.StatusCode = tc.detailStatus
				return x, nil
			}, nil)
			receipt := &absenceReceipt{}
			d.absence = receipt
			_, err := d.GetServer(context.Background(), serverID)
			if providers.IsClass(err, providers.ErrorNotFound) != tc.absent || (receipt.confirms > 0) != tc.proof || (receipt.presents > 0) != tc.present || exact != 1 || full > 1 || basic < 1 {
				t.Fatalf("proof=%+v exact=%d full=%d basic=%d err=%v", receipt, exact, full, basic, err)
			}
		})
	}
}

func TestKamateraAlternateAbsencePreservesDurableFence(t *testing.T) {
	d := fixtureDriver(t, func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" {
			var s map[string]string
			json.NewDecoder(r.Body).Decode(&s)
			if s["id"] != "" {
				x := reply(`{}`)
				x.StatusCode = 500
				return x, nil
			}
		}
		return reply("[]"), nil
	}, nil)
	if _, err := d.GetServer(context.Background(), serverID); err == nil || providers.IsClass(err, providers.ErrorNotFound) {
		t.Fatal("missing durable evidence accepted", err)
	}
	receipt := &absenceReceipt{err: errors.New("durable observation still pending")}
	d.absence = receipt
	for i := 0; i < 3; i++ {
		if _, err := d.GetServer(context.Background(), serverID); !errors.Is(err, receipt.err) {
			t.Fatal("fence bypass", err)
		}
	}
	receipt.err = nil
	if _, err := d.GetServer(context.Background(), serverID); !providers.IsClass(err, providers.ErrorNotFound) || receipt.confirms != 4 {
		t.Fatal("stable absence missing", err)
	}
}
