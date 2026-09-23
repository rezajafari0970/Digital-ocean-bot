package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"net/http"
	"time"
)

type proxyObservation struct {
	IP          string `json:"ip"`
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	ASN         string `json:"asn"`
	LatencyMS   int64  `json:"latency_ms"`
}

func observeProxy(ctx context.Context, x proxyWrite, typ network.ProxyType) (proxyObservation, error) {
	p := network.Proxy{Name: x.Name, Type: typ, Host: x.Host, Port: x.Port, Status: network.StatusHealthy}
	g, err := network.NewProxyGateway("observe", p, network.ProxyCredentials{Username: x.Username, Password: x.Password})
	if err != nil {
		return proxyObservation{}, err
	}
	defer g.CloseIdleConnections()
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipwho.is/", nil)
	resp, err := g.Client.Do(req)
	if err != nil {
		return proxyObservation{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return proxyObservation{}, errors.New("geo lookup failed")
	}
	var v struct {
		IP          string `json:"ip"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
		Connection  struct {
			ASN int    `json:"asn"`
			Org string `json:"org"`
		} `json:"connection"`
	}
	if json.NewDecoder(resp.Body).Decode(&v) != nil || v.IP == "" {
		return proxyObservation{}, errors.New("invalid geo response")
	}
	asn := v.Connection.Org
	if v.Connection.ASN > 0 {
		asn = "AS" + fmt.Sprint(v.Connection.ASN) + " " + asn
	}
	return proxyObservation{IP: v.IP, Country: v.Country, CountryCode: v.CountryCode, ASN: asn, LatencyMS: time.Since(start).Milliseconds()}, nil
}
