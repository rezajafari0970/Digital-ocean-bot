package vultrconsole

import "testing"

func TestMaximumInstancesPattern(t *testing.T) {
	cases := map[string]string{
		"Maximum Instances 10":   "10",
		"Maximum Instances:\n25": "25",
		"maximum instance 3":     "3",
	}
	for input, want := range cases {
		m := limitRE.FindStringSubmatch(input)
		if len(m) != 2 || m[1] != want {
			t.Fatalf("%q => %#v, want %s", input, m, want)
		}
	}
}
