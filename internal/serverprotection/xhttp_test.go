package serverprotection

import (
	"reflect"
	"strings"
	"testing"
)

func TestXHTTPListenerKeepsTCPProtection(t *testing.T) {
	ports, e := ParsePorts(strings.NewReader(`{"inbounds":[{"protocol":"vless","port":443,"streamSettings":{"network":"xhttp","security":"reality"}},{"protocol":"vless","port":8443,"listen":"127.0.0.1","streamSettings":{"network":"xhttp"}},{"protocol":"vless","port":22,"streamSettings":{"network":"xhttp"}}]}`), nil)
	if e != nil || !reflect.DeepEqual(ports, []int{443}) {
		t.Fatal(ports, e)
	}
}
