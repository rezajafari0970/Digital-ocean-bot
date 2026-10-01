package vultr

import (
	"encoding/json"
	"testing"
)

func TestCreateInstanceRequestUsesCurrentSSHKeyIDsField(t *testing.T) {
	b, err := json.Marshal(createInstanceRequest{Region: "ewr", Plan: "vc2", OSID: 2284, SSHKeyIDs: []string{"key-1"}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["ssh_key_ids"]; !ok {
		t.Fatalf("missing ssh_key_ids in %s", b)
	}
	if _, legacy := got["sshkey_id"]; legacy {
		t.Fatalf("legacy sshkey_id must not be emitted: %s", b)
	}
}
