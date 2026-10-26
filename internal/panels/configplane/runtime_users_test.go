package configplane

import (
	"os/exec"
	"testing"
)

func TestNativeVLESSIdentityAndFlowProof(t *testing.T) {
	payload := map[string]any{"protocol": "vless", "settings": map[string]any{"clients": []any{
		map[string]any{"email": "owned", "id": "fixture-id", "flow": "xtls-rprx-vision"},
	}}}
	users, e := expectedNativeVLESSUsers(payload)
	if e != nil || len(users) != 1 || users[0].ID != "fixture-id" || users[0].Flow != "xtls-rprx-vision" {
		t.Fatal(users, e)
	}
	payload["settings"].(map[string]any)["clients"] = []any{map[string]any{"email": "owned", "id": "one"}, map[string]any{"email": "owned", "id": "two"}}
	if _, e = expectedNativeVLESSUsers(payload); e == nil {
		t.Fatal("ambiguous desired identities accepted")
	}
	script := `
import pathlib
ns={};exec(pathlib.Path('runtime_users.py').read_text(),ns)
check=ns['native_users_match']
expected=[{'email':'owned','id':'one','flow':'xtls-rprx-vision'}]
want={'emails':['owned'],'vless_users':expected}
good=[{'email':'owned','account':{'id':'one','flow':'xtls-rprx-vision'}}]
assert check(good,want)
for bad in [
 [{'email':'owned','account':{'id':'one','flow':''}}],
 [{'email':'owned','account':{'id':'other','flow':'xtls-rprx-vision'}}],
 [{'email':'owned'}],
 good+good,
 []]:
 assert not check(bad,want),bad
# Missing Flow is the empty Flow, never Vision; non-VLESS keeps email proof.
assert check([{'email':'owned','account':{'id':'one'}}],{'emails':['owned'],'vless_users':[dict(expected[0],flow='')]})
assert check([{'email':'owned'}],{'emails':['owned'],'vless_users':None})
assert not check([{'email':'other'}],{'emails':['owned'],'vless_users':None})
`
	if out, e := exec.Command("python3", "-c", script).CombinedOutput(); e != nil {
		t.Fatalf("%v %s", e, out)
	}
}
