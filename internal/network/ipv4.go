package network

import (
	"context"
	"errors"
	"net"
	"strconv"
	"time"
)

var ErrIPv4Required = errors.New("IPv4 required")

func ResolveIPv4(ctx context.Context, host string) (string, error) {
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			return v4.String(), nil
		}
		return "", ErrIPv4Required
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if err != nil {
		return "", err
	}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return v4.String(), nil
		}
	}
	return "", ErrIPv4Required
}
func DialContextIPv4(ctx context.Context, networkName, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ip, err := ResolveIPv4(ctx, host)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return d.DialContext(ctx, "tcp4", net.JoinHostPort(ip, port))
}
func IPv4Endpoint(ctx context.Context, host string, port int) (string, error) {
	ip, err := ResolveIPv4(ctx, host)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(ip, strconv.Itoa(port)), nil
}

type ipv4ProxyDialer struct{}

func (*ipv4ProxyDialer) Dial(networkName, address string) (net.Conn, error) {
	return DialContextIPv4(context.Background(), networkName, address)
}
