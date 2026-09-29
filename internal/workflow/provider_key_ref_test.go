package workflow

import (
	"encoding/json"
	"testing"
)

func TestProviderKeyRefReadsHistoricalNumberAndNewString(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{{`{"ssh_provider_key_id":123}`, "123"}, {`{"ssh_provider_key_id":"abc-123"}`, "abc-123"}} {
		var p ProfileSnapshot
		if err := json.Unmarshal([]byte(tc.raw), &p); err != nil {
			t.Fatal(err)
		}
		if p.SSHProviderKeyID.String() != tc.want {
			t.Fatalf("got=%q want=%q", p.SSHProviderKeyID, tc.want)
		}
	}
}
