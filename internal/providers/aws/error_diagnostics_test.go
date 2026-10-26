package aws

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
	"strings"
	"testing"
)

func TestAWSErrorClassificationPreservesActionableCause(t *testing.T) {
	for _, tc := range []struct {
		code  string
		class providers.ErrorClass
	}{{"IncompleteSignature", providers.ErrorAuthentication}, {"MissingAuthenticationToken", providers.ErrorAuthentication}, {"InvalidSignatureException", providers.ErrorAuthentication}, {"Blocked", providers.ErrorAccountLocked}, {"AccountDisabled", providers.ErrorAccountLocked}, {"InvalidParameterValue", providers.ErrorInvalidRequest}, {"InvalidQueryParameter", providers.ErrorInvalidRequest}, {"OptInRequired", providers.ErrorPermissionDenied}} {
		t.Run(tc.code, func(t *testing.T) {
			e := classify("DescribeRegions", 400, []byte(`<Response><Errors><Error><Code>`+tc.code+`</Code><Message>private-secret</Message></Error></Errors></Response>`))
			if e.Class != tc.class {
				t.Fatalf("class %s want %s", e.Class, tc.class)
			}
			if strings.Contains(e.Error(), "private-secret") {
				t.Fatal("raw body leaked")
			}
		})
	}
}
func TestAWSNonProviderErrorNeverBlamesCredentials(t *testing.T) {
	for _, raw := range []string{`<html>private-secret</html>`, `not XML private-secret`, `<Response><Errors><Error><Code>AuthFailure</Code></Error></Errors>`} {
		e := classify("DescribeRegions", 400, []byte(raw))
		if e.Class != providers.ErrorUnavailable {
			t.Fatalf("non-provider reply classified %s", e.Class)
		}
	}
}

func TestAWSDiagnosticsAllowlistBothEnvelopes(t *testing.T) {
	for _, outer := range []string{"ec2", "sts"} {
		raw := `<Response><Errors><Error><Code>InvalidParameterValue</Code><Message>private-secret</Message></Error></Errors></Response>`
		if outer == "sts" {
			raw = `<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><Error><Type>Sender</Type><Code>InvalidParameterValue</Code><Message>private-secret</Message></Error><RequestId>private-secret</RequestId></ErrorResponse>`
		}
		e := classify("DescribeRegions", 400, []byte(raw))
		d := Diagnostic(e)
		if d.Code != "InvalidParameterValue" || d.Operation != "DescribeRegions" || d.RequiredPermission != "" || d.ResponseFormat != outer+"_xml" {
			t.Fatalf("lost diagnostic: %+v", d)
		}
		if strings.Contains(d.Guidance, "private-secret") || strings.Contains(e.Error(), "private-secret") {
			t.Fatal("secret leak")
		}
	}
	e := classify("private-secret", 400, []byte(`<Response><Errors><Error><Code>private-secret</Code></Error></Errors></Response>`))
	d := Diagnostic(e)
	if d.Code != "UNRECOGNIZED_AWS_ERROR" || d.Operation != "unknown" {
		t.Fatalf("unknown metadata escaped: %+v", d)
	}
	d = Diagnostic(&providers.Error{Code: "private-secret", Operation: "private-secret", Message: "private-secret"})
	if d.Code != "UNKNOWN" || d.Operation != "unknown" {
		t.Fatal("injected metadata escaped")
	}
}

func TestAWSBackgroundErrorRetainsSafeWireCause(t *testing.T) {
	e := classify("GetCallerIdentity", 403, []byte(`<ErrorResponse><Error><Code>InvalidClientTokenId</Code><Message>private-secret</Message></Error></ErrorResponse>`))
	if !strings.Contains(e.Error(), "code=InvalidClientTokenId") || !strings.Contains(e.Error(), "operation=GetCallerIdentity") || strings.Contains(e.Error(), "private-secret") {
		t.Fatal("background diagnostic lost safe cause", e)
	}
	e = classify("private-secret", 403, []byte(`<ErrorResponse><Error><Code>private-secret</Code><Message>private-secret</Message></Error></ErrorResponse>`))
	if strings.Contains(e.Error(), "private-secret") || !strings.Contains(e.Error(), "code=UNRECOGNIZED_AWS_ERROR") || !strings.Contains(e.Error(), "operation=unknown") {
		t.Fatal("untrusted wire metadata escaped", e)
	}
}
