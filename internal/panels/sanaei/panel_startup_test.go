package sanaei

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPanelReadinessWaitsForColdStartAndStopsWhenUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name       string
		readyAfter int
		success    bool
	}{{"cold-start", 3, true}, {"unavailable", 99, false}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			scripts := map[string]string{
				"systemctl": "#!/bin/sh\n[ \"$1\" = is-active ]\n",
				"sleep":     "#!/bin/sh\nexit 0\n",
				"curl":      "#!/bin/sh\nn=0; test ! -f \"$TEST_COUNT\" || n=$(cat \"$TEST_COUNT\"); n=$((n+1)); echo $n > \"$TEST_COUNT\"; test \"$n\" -ge \"$TEST_READY\"\n",
			}
			for n, s := range scripts {
				if err := os.WriteFile(filepath.Join(dir, n), []byte(s), 0700); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("bash", "-c", panelReadinessCommand())
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "XUI_PATH=/test/", "XUI_PORT=2053", "TEST_COUNT="+filepath.Join(dir, "count"), "TEST_READY="+func() string {
				if tc.success {
					return "3"
				}
				return "99"
			}())
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.success {
				t.Fatal(string(out), err)
			}
			raw, err := os.ReadFile(filepath.Join(dir, "count"))
			if err != nil {
				t.Fatal(err)
			}
			want := "15"
			if tc.success {
				want = "3"
			}
			if strings.TrimSpace(string(raw)) != want {
				t.Fatal(string(raw), want)
			}
		})
	}
}
