package listeners

import (
	"context"
	"reflect"
	"testing"
)

func TestSSHCollector(t *testing.T) {
	c := SSHCollector{Run: func(context.Context, string) (string, error) {
		return "LISTEN 0 128 0.0.0.0:22 0.0.0.0:*\nLISTEN 0 128 *:2053 *:*\n", nil
	}}
	p, e := c.Ports(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(p, []int{22, 2053}) {
		t.Fatalf("%v", p)
	}
}
