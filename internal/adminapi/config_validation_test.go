package adminapi

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestGlobalConfigInvalidRateExplainedBeforeAnyDatabaseAccess(t *testing.T) {
	for _, rate := range []int{0, -1, 101, 600} {
		x := globalConfigRequest{Ports: []int{443}, TargetUsersPerInbound: 1, UsersPerSecond: rate, SNISelectionMode: "scored"}
		b, _ := json.Marshal(x)
		w := httptest.NewRecorder()
		(&Server{}).putGlobalConfig(w, httptest.NewRequest("PUT", "/api/v1/configs", bytes.NewReader(b)))
		var got map[string]string
		if json.Unmarshal(w.Body.Bytes(), &got) != nil || w.Code != 400 || got["field"] != "users_per_second" || got["detail"] == "" {
			t.Fatalf("rate %d: status %d body %s", rate, w.Code, w.Body.String())
		}
	}
}
func TestGlobalConfigRateBoundaries(t *testing.T) {
	for _, rate := range []int{1, 100} {
		_, detail := validateGlobalConfig(globalConfigRequest{Ports: []int{443}, TargetUsersPerInbound: 2, UsersPerSecond: rate, SNISelectionMode: "scored"})
		if detail != "" {
			t.Fatalf("valid rate %d: %s", rate, detail)
		}
	}
}
