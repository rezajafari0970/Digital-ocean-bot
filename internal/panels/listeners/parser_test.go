package listeners

import (
	"reflect"
	"testing"
)

func TestParseSS(t *testing.T) {
	s := "LISTEN 0 4096 0.0.0.0:22 0.0.0.0:*\nLISTEN 0 4096 [::]:2053 [::]:*\n"
	if g := ParseSS(s); !reflect.DeepEqual(g, []int{22, 2053}) {
		t.Fatalf("%v", g)
	}
}
