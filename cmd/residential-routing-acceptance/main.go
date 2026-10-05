// Read-only acceptance by default. A UUID followed by --apply performs one canary with the worker inactive.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/app"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/readyworker"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/residentialsync"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

func object(raw json.RawMessage) (map[string]any, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		raw = []byte(s)
	}
	var v map[string]any
	err := json.Unmarshal(raw, &v)
	if v == nil {
		return nil, fmt.Errorf("empty object")
	}
	return v, err
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	a, e := app.Bootstrap(ctx)
	if e != nil {
		return fmt.Errorf("bootstrap unavailable")
	}
	defer a.Close()
	q := `SELECT DISTINCT p.id::text FROM panel_instances p JOIN droplets d ON d.id=p.droplet_id JOIN deployments dep ON dep.droplet_id=d.id WHERE p.enabled AND d.state='READY' AND dep.state='PANEL_COMPLETE' AND (d.expires_at IS NULL OR d.expires_at>now()+interval '60 seconds')`
	var args []any
	if len(os.Args) > 1 {
		q += " AND p.id=$1"
		args = append(args, os.Args[1])
	}
	q += " ORDER BY p.id::text"
	rows, e := a.DB.QueryContext(ctx, q, args...)
	if e != nil {
		return e
	}
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if len(ids) == 0 {
		return fmt.Errorf("no serving panels")
	}
	m := &sanaei.RuntimeManager{Factory: sanaei.RuntimeFactory{DB: a.DB, Secrets: a.Container.Secrets, Timeout: 8 * time.Second}, TTL: time.Second}
	if len(os.Args) > 2 {
		if len(os.Args) != 3 || os.Args[2] != "--apply" || len(ids) != 1 {
			return fmt.Errorf("invalid apply scope")
		}
		out, e := exec.Command("systemctl", "is-active", "digital-ocean-bot-worker").Output()
		if e == nil || strings.TrimSpace(string(out)) != "inactive" {
			return fmt.Errorf("worker must be inactive for canary")
		}
		deadline := time.Now().Add(35 * time.Second)
		for {
			var due bool
			e = a.DB.QueryRowContext(ctx, "SELECT c.enabled AND (c.fleet OR $1::uuid=ANY(c.panel_ids)) AND (r.panel_id IS NULL OR r.revision<>c.revision OR r.next_check_at<=now()) FROM residential_routing_control c LEFT JOIN panel_routing_state r ON r.panel_id=$1 WHERE c.singleton", ids[0]).Scan(&due)
			if e != nil {
				return e
			}
			if due {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("scope closed or not due")
			}
			time.Sleep(time.Second)
		}
		svc := residentialsync.Service{DB: a.DB, Secrets: a.Container.Secrets, Runtimes: m}
		if e = svc.ReconcilePanel(ctx, readyworker.Panel{ID: ids[0]}, false); e != nil {
			return fmt.Errorf("canary unconfirmed: %w", e)
		}
	}
	passed, failed := 0, 0
	for _, id := range ids {
		verify := func(ctx context.Context) (int, error) {
			rt, e := m.Acquire(ctx, id)
			if e != nil {
				return 0, fmt.Errorf("runtime unavailable")
			}
			response, e := rt.Session.Exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: "panel/api/xray/", TimeoutSeconds: 8})
			if e != nil {
				return 0, fmt.Errorf("settings unavailable")
			}
			var top struct {
				Success bool
				Obj     json.RawMessage
			}
			if json.Unmarshal(response.Body, &top) != nil || !top.Success {
				return 0, fmt.Errorf("settings envelope")
			}
			obj, e := object(top.Obj)
			if e != nil {
				return 0, e
			}
			raw, _ := json.Marshal(obj["xraySetting"])
			setting, e := object(raw)
			if e != nil {
				return 0, e
			}
			dns, ok := setting["dns"].(map[string]any)
			if !ok {
				return 0, fmt.Errorf("DNS missing")
			}
			servers, ok := dns["servers"].([]any)
			if !ok || len(servers) != 2 {
				return 0, fmt.Errorf("DNS pool count")
			}
			for i, wanted := range []string{"tcp://1.1.1.1:53", "tcp://8.8.8.8:53"} {
				if servers[i] != wanted {
					return 0, fmt.Errorf("DNS differs from approved providers")
				}
			}
			routing, ok := setting["routing"].(map[string]any)
			if !ok {
				return 0, fmt.Errorf("routing missing")
			}
			rules, ok := routing["rules"].([]any)
			if !ok {
				return 0, fmt.Errorf("rules missing")
			}
			protected, direct, blocked, clientDNS := "", "", "", ""
			poolAllowed, e := poolTargets(setting)
			if e != nil {
				return 0, e
			}
			poolScope := map[string]bool{}
			if dns["queryStrategy"] != "UseIPv4" || dns["disableCache"] != false {
				return 0, fmt.Errorf("DNS IPv4/cache policy differs")
			}
			for _, item := range rules {
				r, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if r["ruleTag"] == "dob-route-residential-ads" || r["ruleTag"] == "dob-route-residential-ads-tcp" || r["ruleTag"] == "dob-route-residential-ads-udp" {
					domains, _ := r["domain"].([]any)
					wanted := []string{"geosite:google@ads", "domain:browserleaks.com"}
					netw, _ := r["network"].(string)
					if len(domains) != len(wanted) || (netw != "tcp,udp" && netw != "tcp" && netw != "udp") {
						return 0, fmt.Errorf("Ads-only scope differs")
					}
					for i, d := range wanted {
						if domains[i] != d {
							return 0, fmt.Errorf("Ads categories differ")
						}
					}
					protected, _ = r["outboundTag"].(string)
					if netw == "tcp" || netw == "udp" {
						poolScope[netw] = true
						if r["balancerTag"] == "dob-route-pool-"+netw {
							protected = "@pool"
						} else if len(poolAllowed["@pool:"+netw]) > 1 {
							return 0, fmt.Errorf("pool routing bypass")
						}
					}
				}
				if r["ruleTag"] == "dob-route-default" {
					direct, _ = r["outboundTag"].(string)
				}
				if r["ruleTag"] == "dob-route-unobserved-inbound" {
					blocked, _ = r["outboundTag"].(string)
				}
				if r["ruleTag"] == "dob-route-client-dns" {
					clientDNS, _ = r["outboundTag"].(string)
				}
				if r["ruleTag"] == "dob-route-residential-udp" && r["domain"] == nil {
					return 0, fmt.Errorf("overbroad UDP block")
				}
				if r["ruleTag"] == "dob-route-known-non-ad" {
					return 0, fmt.Errorf("obsolete rule retained")
				}
			}
			if len(poolAllowed) > 0 {
				protected = "@pool"
			}
			if protected != "@pool" && !strings.HasPrefix(protected, "residential-ads-") && !strings.HasPrefix(protected, "dob-route-blocked-") || !strings.HasPrefix(direct, "dob-route-direct-") || !strings.HasPrefix(blocked, "dob-route-blocked-") || !strings.HasPrefix(clientDNS, "dob-route-client-dns-resolver-") {
				return 0, fmt.Errorf("class route unavailable")
			}

			if len(poolAllowed) > 0 && (!poolScope["tcp"] || !poolScope["udp"]) {
				return 0, fmt.Errorf("pool protocol scope incomplete")
			}
			rt.Session.Invalidate()
			inbounds, e := rt.Session.Snapshot(ctx)
			if e != nil {
				return 0, fmt.Errorf("fresh inbound unavailable")
			}
			type client struct{ ID, Email string }
			tag := ""
			observed := map[string]bool{}
			for _, raw := range inbounds {
				var in struct {
					Port     int
					Tag      string
					Settings json.RawMessage
					Sniffing json.RawMessage
				}
				if json.Unmarshal(raw, &in) != nil {
					return 0, fmt.Errorf("inbound shape")
				}
				if in.Port != 443 {
					continue
				}
				tag = in.Tag
				sn, e := object(in.Sniffing)
				if e != nil || sn["enabled"] != true || sn["metadataOnly"] == true || sn["routeOnly"] == true {
					return 0, fmt.Errorf("payload sniffing/hostname override unavailable")
				}
				s, e := object(in.Settings)
				if e != nil {
					return 0, e
				}
				v, _ := json.Marshal(s)
				var clients struct{ Clients []client }
				json.Unmarshal(v, &clients)
				for _, c := range clients.Clients {
					observed[c.ID+"|"+c.Email] = true
				}
			}
			if tag == "" {
				return 0, fmt.Errorf("port443 unavailable")
			}
			rr, e := a.DB.QueryContext(ctx, `SELECT client_id,email,route_class FROM panel_client_routes WHERE panel_id=$1 ORDER BY client_id`, id)
			if e != nil {
				return 0, e
			}
			emails := map[string]string{}
			for rr.Next() {
				var cid, email, cls string
				if e = rr.Scan(&cid, &email, &cls); e != nil {
					rr.Close()
					return 0, e
				}
				if observed[cid+"|"+email] && emails[cls] == "" {
					emails[cls] = email
				}
			}
			e = rr.Err()
			rr.Close()
			if e != nil {
				return 0, e
			}
			if emails["DIRECT"] == "" || emails["RESIDENTIAL"] == "" {
				return 0, fmt.Errorf("both observed route classes not ready")
			}
			count := 0
			check := func(form url.Values, expected string) error {
				response, e := rt.Session.Exec.Do(ctx, sanaei.SessionRequest{Method: "POST", Path: "panel/api/xray/routeTest", ContentType: "application/x-www-form-urlencoded", Body: []byte(form.Encode()), TimeoutSeconds: 5})
				if e != nil {
					return fmt.Errorf("route proof unavailable")
				}
				wanted := []string{expected}
				if expected == "@pool" {
					wanted = poolAllowed["@pool:"+form.Get("network")]
				} else if strings.HasPrefix(expected, "@inner:") {
					wanted = poolAllowed[expected]
				}
				matches := false
				for _, candidate := range wanted {
					ok, err := routeProofMatches(response, candidate)
					if err != nil {
						return err
					}
					matches = matches || ok
				}
				if e != nil {
					return e
				}
				if !matches {
					return fmt.Errorf("runtime differs network=%s destination=%s%s port=%s", form.Get("network"), form.Get("domain"), form.Get("ip"), form.Get("port"))
				}
				count++
				return nil
			}
			for _, cls := range []string{"DIRECT", "RESIDENTIAL"} {
				for _, network := range []string{"tcp", "udp"} {
					for _, dest := range []struct {
						domain, ip, port string
						ad               bool
					}{
						{"adservice.google.com", "", "443", true}, {"pixel.facebook.com", "", "443", false}, {"browserleaks.com", "", "443", true}, {"tls.browserleaks.com", "", "443", true}, {"browserleaks.com.example.org", "", "443", false}, {"www.example.com", "", "443", false}, {"", "1.1.1.1", "443", false}, {"", "1.1.1.1", "53", false},
					} {
						expected := direct
						if cls == "RESIDENTIAL" {
							if dest.port == "53" {
								expected = clientDNS
							} else if dest.ad {
								expected = protected
							}
						}
						form := url.Values{"inboundTag": {tag}, "email": {emails[cls]}, "network": {network}, "port": {dest.port}}
						if dest.domain != "" {
							form.Set("domain", dest.domain)
						} else {
							form.Set("ip", dest.ip)
						}
						if e = check(form, expected); e != nil {
							return 0, e
						}
					}
				}
			}
			if e = check(url.Values{"inboundTag": {"dob-route-dns-query"}, "network": {"tcp"}, "port": {"53"}, "ip": {"8.8.8.8"}}, direct); e != nil {
				return 0, e
			}
			if e = check(url.Values{"inboundTag": {"dob-unobserved-inbound"}, "network": {"tcp"}, "port": {"443"}, "domain": {"adservice.google.com"}}, blocked); e != nil {
				return 0, e
			}
			for _, network := range []string{"tcp", "udp"} {
				for _, kind := range []string{"fast", "all"} {
					key := "@inner:" + network + ":" + kind
					if len(poolAllowed[key]) > 0 {
						if e = check(url.Values{"inboundTag": {"dob-route-pool-in-" + network + "-" + kind}, "network": {network}, "port": {"443"}, "ip": {"1.1.1.1"}}, key); e != nil {
							return 0, e
						}
					}
				}
			}
			return count, nil
		}
		count, err := verifyUnderConfigLock(ctx, a.DB, id, verify)
		if err != nil {
			failed++
			fmt.Printf("ADS_UDP_DNS_PENDING panel=%s reason=%v\n", id, err)
		} else {
			passed++
			fmt.Printf("ADS_UDP_DNS_PASS panel=%s route_proofs=%d\n", id, count)
		}
	}
	fmt.Printf("ADS_UDP_DNS_FLEET_PROOF panels=%d passed=%d pending=%d at=%s\n", len(ids), passed, failed, time.Now().UTC().Format(time.RFC3339))
	if failed > 0 {
		return fmt.Errorf("some serving panels not yet verified")
	}
	return nil
}
