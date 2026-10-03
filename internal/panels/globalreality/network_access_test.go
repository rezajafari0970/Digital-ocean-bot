package globalreality

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopedFirewallPlanValidatesPortsAndRecoversUnknownOutcome(t *testing.T) {
	for _, ports := range [][]int{{22}, {2053}, {0}, {65536}} {
		if _, err := networkAccessCommand(2053, ports); err == nil {
			t.Fatal("unsafe port admitted", ports)
		}
	}
	cmd, err := networkAccessCommand(2053, []int{443, 8443, 443})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	rules := filepath.Join(dir, "rules")
	fail := filepath.Join(dir, "fail")
	fake := `#!/usr/bin/python3
import sys,os,json,shlex
f=os.environ['TEST_RULES'];a=sys.argv[1:]
rules=json.load(open(f)) if os.path.exists(f) else []
if a==['status']: print('Status: active')
elif a==['show','added']:
 for rule in rules: print(shlex.join(['ufw',*rule]))
elif a and a[0]=='allow':
 if a not in rules: rules.append(a)
 json.dump(rules,open(f,'w'))
 if not os.path.exists(os.environ['TEST_FAIL']):
  open(os.environ['TEST_FAIL'],'w').close()
  sys.exit(1) # rule committed, response lost
else: sys.exit(19)
`
	if err = os.WriteFile(filepath.Join(dir, "ufw"), []byte(fake), 0700); err != nil {
		t.Fatal(err)
	}
	run := func() ([]byte, error) {
		c := exec.Command("bash", "-c", cmd)
		c.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "SSH_CONNECTION=198.51.100.7 50000 203.0.113.9 22", "TEST_RULES="+rules, "TEST_FAIL="+fail)
		return c.CombinedOutput()
	}
	if _, err = run(); err == nil {
		t.Fatal("fault not exercised")
	}
	if out, err := run(); err != nil {
		t.Fatal(string(out), err)
	}
	before, _ := os.ReadFile(rules)
	if out, err := run(); err != nil {
		t.Fatal(string(out), err)
	}
	after, _ := os.ReadFile(rules)
	if string(before) != string(after) {
		t.Fatal("duplicate rules after retry")
	}
	if !strings.Contains(string(after), "198.51.100.7") || strings.Contains(cmd, "--force disable") || strings.Contains(cmd, "firewalld --stop") {
		t.Fatal("management boundary")
	}
}
