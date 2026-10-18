package configplane

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei"
	"testing"
)

func TestProfileSNISelectionRequiresVerifiedPublicTLS(t *testing.T) {
	spec := configdriver.Spec{Protocol: "vless", Network: "tcp", Security: "reality", Port: 443, SNIMode: "auto"}
	good := sanaei.RealityScanResult{Host: "example.com", Port: 443, Feasible: true, CertValid: true, CertChainValid: true, TLSVersion: "1.3", ALPN: "h2", LatencyMS: 20}
	for _, mutate := range []func(*sanaei.RealityScanResult){func(x *sanaei.RealityScanResult) { x.PrivateTarget = true }, func(x *sanaei.RealityScanResult) { x.CertChainValid = false }, func(x *sanaei.RealityScanResult) { x.CertValid = false }, func(x *sanaei.RealityScanResult) { x.TLSVersion = "1.2" }, func(x *sanaei.RealityScanResult) { x.ALPN = "http/1.1" }, func(x *sanaei.RealityScanResult) { x.Port = 8443 }, func(x *sanaei.RealityScanResult) { x.Host = "127.0.0.1" }, func(x *sanaei.RealityScanResult) { x.Feasible = false }} {
		bad := good
		mutate(&bad)
		if _, e := selectSNITarget(spec, []sanaei.RealityScanResult{bad}); e == nil {
			t.Fatal("unsafe SNI accepted", bad)
		}
	}
	faster := good
	faster.Host = "example.org"
	faster.LatencyMS = 10
	target, e := selectSNITarget(spec, []sanaei.RealityScanResult{good, faster})
	if e != nil || target.Host != "example.org" {
		t.Fatal("auto selection", e)
	}
	spec.SNIMode = "manual"
	spec.SNI = "example.com"
	target, e = selectSNITarget(spec, []sanaei.RealityScanResult{faster, good})
	if e != nil || target.Host != spec.SNI {
		t.Fatal("manual SNI overwritten", e)
	}
	spec.SNI = "missing.example"
	if _, e = selectSNITarget(spec, []sanaei.RealityScanResult{good}); e == nil {
		t.Fatal("unverified manual SNI accepted")
	}
}
