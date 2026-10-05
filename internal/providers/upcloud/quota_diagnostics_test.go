package upcloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/providers"
)

func TestQuotaDiagnosticsIdentifyShapeWithoutValues(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"null", "NULL"}, {"-1", "NEGATIVE_NUMBER"}, {"1.5", "DECIMAL_NUMBER"},
		{"1.0", "DECIMAL_NUMBER"}, {"1e3", "EXPONENT_NUMBER"},
		{"1125899906842625", "OUT_OF_RANGE_NUMBER"},
		{`"-2"`, "NEGATIVE_STRING"}, {`"1.5"`, "DECIMAL_STRING"},
		{`"1e3"`, "EXPONENT_STRING"}, {`"ucat_do-not-log"`, "NON_NUMERIC_STRING"},
		{"true", "BOOLEAN"}, {"[]", "ARRAY"}, {"{}", "OBJECT"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			var a accountData
			err := json.Unmarshal([]byte(`{"username":"private-user","resource_limits":{"cores":`+tc.raw+`}}`), &a)
			if err == nil {
				t.Fatal("invalid quota accepted")
			}
			want := "INVALID_QUOTA_CORES_" + tc.want
			if Diagnostic(err) != want {
				t.Fatalf("diagnostic=%s want=%s", Diagnostic(err), want)
			}
			issues := QuotaIssues(err)
			if len(issues) != 1 || issues[0] != want {
				t.Fatal(issues)
			}
			issues[0] = "changed"
			if QuotaIssues(err)[0] != want {
				t.Fatal("returned mutable internal metadata")
			}
			if strings.Contains(err.Error(), "ucat_") || strings.Contains(err.Error(), "private-user") {
				t.Fatal("secret exposed")
			}
		})
	}
}

func TestQuotaDiagnosticsSurviveHTTPNormalization(t *testing.T) {
	d := fixture(t, func(*http.Request) (int, any, error) {
		return 200, map[string]any{"account": map[string]any{"username": "private-user", "resource_limits": map[string]any{
			"memory": nil, "cores": -1, "ucat_private-key": "ucat_private-value",
		}}}, nil
	}, nil)
	_, err := d.Account(context.Background())
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.StatusCode != 200 || pe.Operation != "account" || providers.Class(err) != providers.ErrorUnavailable {
		t.Fatal(err)
	}
	want := []string{"INVALID_QUOTA_CORES_NEGATIVE_NUMBER", "INVALID_QUOTA_MEMORY_NULL", "INVALID_QUOTA_OTHER_FIELD_NON_NUMERIC_STRING"}
	got := QuotaIssues(err)
	if fmt.Sprint(got) != fmt.Sprint(want) || Diagnostic(err) != want[0] {
		t.Fatal(got, Diagnostic(err))
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "ucat_") {
		t.Fatal("raw key or value exposed")
	}
}

func TestQuotaDiagnosticsAreBounded(t *testing.T) {
	limits := map[string]any{}
	for i := 0; i < 200; i++ {
		limits[fmt.Sprintf("secret-key-%d", i)] = true
	}
	raw, _ := json.Marshal(limits)
	var q resourceLimits
	err := json.Unmarshal(raw, &q)
	issues := QuotaIssues(err)
	if len(issues) != 64 || q != nil {
		t.Fatalf("len=%d partial=%v", len(issues), q)
	}
	for _, s := range issues {
		if s != "INVALID_QUOTA_OTHER_FIELD_BOOLEAN" || !safeDiagnostic.MatchString(s) {
			t.Fatal(s)
		}
	}
}

func TestQuotaDiagnosticsKeepPriorAcceptanceRules(t *testing.T) {
	var q resourceLimits
	raw := `{"cores":"8","memory":8192,"cloud_server_dev_1xcpu_1gb_plans":null,"cloud_server_dev_1xcpu_1gb_10gb_plans":0}`
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		t.Fatal(err)
	}
	if q["cloud_server_dev_1xcpu_1gb_plans"] != nil || *q["cloud_server_dev_1xcpu_1gb_10gb_plans"] != 0 || *q["cores"] != 8 {
		t.Fatal(q)
	}
}

func TestQuotaMetadataExportRejectsInjectedFields(t *testing.T) {
	err := &responseError{Code: "INVALID_NUMBER", QuotaIssues: []string{
		"ucat_secret", "INVALID_QUOTA_UCAT_SECRET_NULL", "INVALID_QUOTA_CORES_secret",
		"INVALID_QUOTA_CORES_NULL\\nforged", strings.Repeat("X", 200), "INVALID_QUOTA_MEMORY_NULL",
	}}
	got := QuotaIssues(err)
	if len(got) != 1 || got[0] != "INVALID_QUOTA_MEMORY_NULL" {
		t.Fatal(got)
	}
}
