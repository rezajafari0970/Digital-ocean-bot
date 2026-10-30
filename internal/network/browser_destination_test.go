package network

import (
	"net"
	"testing"
)

func TestBrowserDestinationPublicOnly(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "0.0.0.0", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.100.100.200", "198.19.1.1", "192.0.2.1", "203.0.113.1", "240.0.0.1", "255.255.255.255", "::1", "2606:4700:4700::1111"} {
		if publicBrowserIP(net.ParseIP(ip)) {
			t.Fatal("unsafe destination accepted", ip)
		}
	}
	if !publicBrowserIP(net.ParseIP("1.1.1.1")) {
		t.Fatal("public IPv4 rejected")
	}
}
