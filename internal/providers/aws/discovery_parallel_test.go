package aws

import (
	"context"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAWSDiscoveryBoundsParallelRegionsAndPinsEachSubnet(t *testing.T) {
	regions := []string{"eu-west-1", "eu-west-2", "eu-west-3", "us-west-1", "us-west-2"}
	var mu sync.Mutex
	active, peak := 0, 0
	arrived := make(chan struct{}, 4)
	release := make(chan struct{})
	req := providers.OpenRequest{AccountID: "preview", Credentials: testCredential(`{"access_key_id":"fixture-access","secret_access_key":"fixture-secret"}`), HTTPClient: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Error("preview mutation")
		}
		action := query(r).Get("Action")
		region := strings.Split(r.URL.Host, ".")[1]
		if action == "DescribeRegions" && region != "eu-west-1" {
			mu.Lock()
			active++
			if active > peak {
				peak = active
			}
			mu.Unlock()
			arrived <- struct{}{}
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			mu.Lock()
			active--
			mu.Unlock()
		}
		body := networkFixture(action)
		if action == "DescribeRegions" {
			body = "<regionInfo>"
			for _, region := range regions {
				body += "<item><regionName>" + region + "</regionName><optInStatus>opt-in-not-required</optInStatus></item>"
			}
			body += "</regionInfo>"
		} else {
			idx := 0
			for i, v := range regions {
				if v == region {
					idx = i
				}
			}
			body = strings.ReplaceAll(body, "eu-west-1a", region+"a")
			body = strings.ReplaceAll(body, "subnet-12345678", fmt.Sprintf("subnet-%08d", 12345678+idx))
		}
		return response(action, body), nil
	})}}
	done := make(chan struct{})
	var result providers.Discovery
	var err error
	go func() {
		defer close(done)
		result, err = (Factory{}).Discover(context.Background(), req, map[string]string{"region_2": regions[1], "region_3": regions[2], "region_4": regions[3], "region_5": regions[4]})
	}()
	for i := 0; i < 3; i++ {
		select {
		case <-arrived:
		case <-time.After(2 * time.Second):
			close(release)
			<-done
			t.Fatal("regional work is serial")
		}
	}
	select {
	case <-arrived:
		close(release)
		<-done
		t.Fatal("more than three concurrent regions")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	<-done
	if err != nil || !result.Complete {
		t.Fatal(err)
	}
	cred, err := credentials(context.Background(), result.Credentials)
	if err != nil || len(cred.Networks) != 4 || peak != 3 {
		t.Fatal(err, peak)
	}
	for i, region := range regions[1:] {
		if cred.Networks[region].Subnet != fmt.Sprintf("subnet-%08d", 12345679+i) {
			t.Fatal("cross-region subnet", region)
		}
	}
}
