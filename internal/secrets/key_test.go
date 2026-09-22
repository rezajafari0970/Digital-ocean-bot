package secrets

import (
	"encoding/base64"
	"os"
	"testing"
)

func TestLoadMasterKeyFromEnvironment(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	t.Setenv("MASTER_KEY_FILE", "")
	t.Setenv("MASTER_KEY_B64", base64.StdEncoding.EncodeToString(key))
	got, err := LoadMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(key) {
		t.Fatal("wrong key")
	}
}

func TestLoadMasterKeyFromFile(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(32 - i)
	}
	p := t.TempDir() + "/master.key"
	if err := os.WriteFile(p, []byte(base64.StdEncoding.EncodeToString(key)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MASTER_KEY_FILE", p)
	t.Setenv("MASTER_KEY_B64", "")
	got, err := LoadMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(key) {
		t.Fatal("wrong file key")
	}
}
