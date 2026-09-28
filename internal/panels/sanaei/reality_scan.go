package sanaei

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

var ErrRealityScan = errors.New("reality target scan failed")

type RealityScanResult struct {
	Target         string   `json:"target"`
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	IP             string   `json:"ip"`
	ServerNames    []string `json:"serverNames"`
	TLSVersion     string   `json:"tlsVersion"`
	ALPN           string   `json:"alpn"`
	CurveID        string   `json:"curveID"`
	CertValid      bool     `json:"certValid"`
	CertChainValid bool     `json:"certChainValid"`
	Feasible       bool     `json:"feasible"`
	PrivateTarget  bool     `json:"privateTarget"`
	LatencyMS      int64    `json:"latencyMs"`
	Reason         string   `json:"reason"`
}

func ScanRealityTargets(
	ctx context.Context,
	exec SessionExecutor,
	targets string,
) ([]RealityScanResult, error) {
	if exec == nil {
		return nil, ErrRealityScan
	}

	form := url.Values{}
	if targets != "" {
		form.Set("targets", targets)
	}

	resp, err := exec.Do(ctx, SessionRequest{
		Method:         http.MethodPost,
		Path:           "panel/api/server/scanRealityTargets",
		Body:           []byte(form.Encode()),
		ContentType:    "application/x-www-form-urlencoded",
		TimeoutSeconds: 45,
	})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil,
			fmt.Errorf(
				"%w: HTTP %d",
				ErrRealityScan,
				resp.StatusCode,
			)
	}

	var envelope struct {
		Success bool                `json:"success"`
		Obj     []RealityScanResult `json:"obj"`
	}

	if err := json.Unmarshal(resp.Body, &envelope); err != nil {
		body := resp.Body
		if len(body) > 512 {
			body = body[:512]
		}
		return nil,
			fmt.Errorf(
				"%w: invalid JSON: %q",
				ErrRealityScan,
				string(body),
			)
	}
	if !envelope.Success {
		body := resp.Body
		if len(body) > 512 {
			body = body[:512]
		}
		return nil,
			fmt.Errorf(
				"%w: rejected: %q",
				ErrRealityScan,
				string(body),
			)
	}

	return envelope.Obj, nil
}

func BestRealityTarget(xs []RealityScanResult) (RealityScanResult, string, error) {
	for _, x := range xs {
		if !x.Feasible || x.Host == "" || x.Port < 1 || x.TLSVersion != "1.3" || x.ALPN != "h2" || !x.CertValid || !x.CertChainValid || len(x.ServerNames) == 0 {
			continue
		}
		sni := x.Host
		return x, sni, nil
	}
	return RealityScanResult{}, "", ErrRealityScan
}

func FindAndUseRealityTarget(
	ctx context.Context,
	exec SessionExecutor,
) (RealityScanResult, string, error) {
	ranked, err := ScanRealityTargets(ctx, exec, "")
	if err != nil {
		return RealityScanResult{}, "", err
	}
	best, _, err := BestRealityTarget(ranked)
	if err != nil {
		return RealityScanResult{}, "", err
	}
	full, err := ScanRealityTargets(ctx, exec, best.Target)
	if err != nil {
		return RealityScanResult{}, "", err
	}
	if len(full) != 1 {
		return RealityScanResult{}, "", ErrRealityScan
	}
	x := full[0]
	if !x.Feasible || len(x.ServerNames) == 0 {
		return RealityScanResult{}, "", ErrRealityScan
	}
	sni := x.ServerNames[0]
	for _, name := range x.ServerNames {
		if name == x.Host {
			sni = x.Host
			break
		}
	}
	return x, sni, nil
}
