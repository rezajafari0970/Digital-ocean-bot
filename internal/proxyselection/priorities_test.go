package proxyselection

import (
	"reflect"
	"testing"
)

func TestThreePriorityContract(t *testing.T) {
	for _, x := range []struct {
		scalar string
		slots  []string
		want   []string
		bad    bool
	}{
		{"", nil, []string{Auto}, false}, {"US", nil, []string{"US"}, false}, {"", []string{Auto, Auto, Auto}, []string{Auto}, false},
		{"", []string{"de", Auto, "us"}, []string{"DE", "US"}, false}, {"", []string{Auto, "FR", "IT"}, []string{"FR", "IT"}, false},
		{"US", []string{"DE", "US"}, nil, true}, {"", []string{"US", "us"}, nil, true}, {"", []string{"US", "DE", "FR", "IT"}, nil, true}, {"", []string{"ZZ"}, nil, true},
	} {
		got, e := Priorities(x.scalar, x.slots)
		if (e != nil) != x.bad || !x.bad && !reflect.DeepEqual(got, x.want) {
			t.Fatalf("%v/%v = %v %v", x.scalar, x.slots, got, e)
		}
	}
}
