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

type ProxyObservation struct {
	IP          string `json:"ip"`
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	Timezone    string `json:"timezone"`
	ASN         string `json:"asn"`
	LatencyMS   int64  `json:"latency_ms"`
}

func observeProxy(ctx context.Context, x proxyWrite, typ network.ProxyType) (ProxyObservation, error) {
	return ObserveProxy(ctx, x.Name, x.Host, x.Port, x.Username, x.Password, typ)
}

func ObserveProxy(ctx context.Context, name, host string, port int, username, password string, typ network.ProxyType) (ProxyObservation, error) {
	p := network.Proxy{Name: name, Type: typ, Host: host, Port: port, Status: network.StatusHealthy}
	g, err := network.NewProxyGateway("observe", p, network.ProxyCredentials{Username: username, Password: password})
	if err != nil {
		return ProxyObservation{}, err
	}
	defer g.CloseIdleConnections()
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipwho.is/", nil)
	resp, err := g.Client.Do(req)
	if err != nil {
		return ProxyObservation{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ProxyObservation{}, errors.New("geo lookup failed")
	}
	var v struct {
		IP          string `json:"ip"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
		Timezone    struct {
			ID string `json:"id"`
		} `json:"timezone"`
		Connection struct {
			ASN int    `json:"asn"`
			Org string `json:"org"`
		} `json:"connection"`
	}
	if json.NewDecoder(resp.Body).Decode(&v) != nil || v.IP == "" {
		return ProxyObservation{}, errors.New("invalid geo response")
	}
	asn := v.Connection.Org
	if v.Connection.ASN > 0 {
		asn = "AS" + fmt.Sprint(v.Connection.ASN) + " " + asn
	}
	return ProxyObservation{IP: v.IP, Country: v.Country, CountryCode: v.CountryCode, Timezone: v.Timezone.ID, ASN: asn, LatencyMS: time.Since(start).Milliseconds()}, nil
}
