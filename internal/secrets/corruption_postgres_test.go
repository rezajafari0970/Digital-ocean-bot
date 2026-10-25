package secrets

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// Socket-only disposable database supplied by the mandatory gate. This private
// schema models the actual read columns without touching public/live tables.
func secretCorruptionDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("BULK_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(u.Path, "bulk_test") {
		t.Fatal("isolated bulk_test database required")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("secret_corruption_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); admin.Exec("DROP SCHEMA " + schema + " CASCADE"); admin.Close() })
	_, err = db.Exec(`
 CREATE TABLE secrets(account_id text,proxy_id text,probe_id text,id text,kind text,ciphertext bytea,nonce bytea,key_version int);
 CREATE TABLE residential_proxy_secrets(residential_id text,id text,kind text,aad_scope text,ciphertext bytea,nonce bytea,key_version int);
 CREATE TABLE cloudflare_secrets(account_id text,ciphertext bytea,nonce bytea,key_version int);
 CREATE TABLE ip_cleanup_credentials(account_id text,operation_id text,original_id text,kind text,ciphertext bytea,nonce bytea,key_version int);
 CREATE TABLE ip_mutation_intents(id text,account_id text,dispatch_id text);
 `)
	if err != nil {
		t.Fatal(err)
	}
	return db
}
func TestSecretSQLReadersRetainVersionAndRejectCorruption(t *testing.T) {
	db := secretCorruptionDB(t)
	ctx := context.Background()
	oldKey, newKey := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)
	old, err := NewStore(&memRepo{}, oldKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := NewStoreWithKeys(SQLRepository{DB: db}, newKey, 2, map[int][]byte{1: oldKey})
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("retained-fixture-value")
	nonce := bytes.Repeat([]byte{9}, old.aead.NonceSize())
	cases := []struct {
		name, table, insert, aad string
		read                     func() ([]byte, error)
	}{
		{"account", "secrets", "INSERT INTO secrets(account_id,id,kind,ciphertext,nonce,key_version) VALUES('owner','key','private',$1,$2,1)", "owner\x00key\x00private", func() ([]byte, error) { return rotated.Get(ctx, "owner", "key") }},
		{"proxy", "secrets", "INSERT INTO secrets(proxy_id,id,kind,ciphertext,nonce,key_version) VALUES('owner','key','private',$1,$2,1)", "proxy\x00owner\x00key\x00private", func() ([]byte, error) { return rotated.GetProxy(ctx, "owner", "key") }},
		{"probe", "secrets", "INSERT INTO secrets(probe_id,id,kind,ciphertext,nonce,key_version) VALUES('owner','key','private',$1,$2,1)", "probe\x00owner\x00key\x00private", func() ([]byte, error) { return rotated.GetProbe(ctx, "owner", "key") }},
		{"residential", "residential_proxy_secrets", "INSERT INTO residential_proxy_secrets(residential_id,id,kind,aad_scope,ciphertext,nonce,key_version) VALUES('owner','key','private','residential',$1,$2,1)", "residential\x00owner\x00key\x00private", func() ([]byte, error) { return rotated.GetResidential(ctx, "owner", "key") }},
		{"cloudflare", "cloudflare_secrets", "INSERT INTO cloudflare_secrets(account_id,ciphertext,nonce,key_version) VALUES('owner',$1,$2,1)", "cloudflare\x00owner", func() ([]byte, error) { return rotated.GetCloudflare(ctx, "owner") }},
		{"cleanup", "ip_cleanup_credentials", "INSERT INTO ip_cleanup_credentials(account_id,operation_id,original_id,kind,ciphertext,nonce,key_version) VALUES('owner','operation','key','private',$1,$2,1)", "owner\x00key\x00private", func() ([]byte, error) { return rotated.GetIPCleanup(ctx, "owner", "operation") }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err = db.Exec("TRUNCATE secrets,residential_proxy_secrets,cloudflare_secrets,ip_cleanup_credentials,ip_mutation_intents"); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec("INSERT INTO ip_mutation_intents VALUES('operation','owner','sent')"); err != nil {
				t.Fatal(err)
			}
			ct := old.aead.Seal(nil, nonce, plain, []byte(test.aad))
			if _, err = db.Exec(test.insert, ct, nonce); err != nil {
				t.Fatal(err)
			}
			if out, e := secretReadWithoutPanic(t, test.read); e != nil || !bytes.Equal(out, plain) {
				t.Fatalf("retained-version read failed: %v", e)
			}
			for _, n := range []int{0, 1, 13} {
				if _, err = db.Exec("UPDATE "+test.table+" SET nonce=$1", make([]byte, n)); err != nil {
					t.Fatal(err)
				}
				if out, e := secretReadWithoutPanic(t, test.read); e == nil || len(out) != 0 {
					t.Fatal("corrupt record returned plaintext")
				}
				if test.name == "cleanup" {
					_, e := secretReadWithoutPanic(t, func() ([]byte, error) { return nil, rotated.ValidateIPCleanupKeys(ctx) })
					if e == nil {
						t.Fatal("retirement validator accepted corrupted archive")
					}
				}
			}
			if _, err = db.Exec("UPDATE "+test.table+" SET nonce=$1,key_version=999", nonce); err != nil {
				t.Fatal(err)
			}
			if out, e := secretReadWithoutPanic(t, test.read); e == nil || len(out) != 0 {
				t.Fatal("unknown version returned plaintext")
			}
			if _, err = db.Exec("UPDATE "+test.table+" SET key_version=1,ciphertext=$1", []byte("broken")); err != nil {
				t.Fatal(err)
			}
			if out, e := secretReadWithoutPanic(t, test.read); e == nil || len(out) != 0 {
				t.Fatal("tampered ciphertext returned plaintext")
			}
		})
	}
}
