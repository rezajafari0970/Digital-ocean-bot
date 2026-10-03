package healthverify

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func ParseVLESSReality(raw string) (RealityClient, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "vless" || u.User == nil {
		return RealityClient{}, ErrInvalid
	}
	uuid := u.User.Username()
	host := u.Hostname()
	p, err := strconv.Atoi(u.Port())
	if err != nil || p < 1 || p > 65535 || net.ParseIP(host) == nil {
		return RealityClient{}, ErrInvalid
	}
	q := u.Query()
	if !strings.EqualFold(q.Get("security"), "reality") {
		return RealityClient{}, errors.New("health uri is not reality")
	}
	sni := q.Get("sni")
	pbk := q.Get("pbk")
	sid := q.Get("sid")
	if uuid == "" || sni == "" || pbk == "" || sid == "" {
		return RealityClient{}, ErrInvalid
	}
	return RealityClient{
		UUID: uuid, Host: host, Port: p, SNI: sni,
		PublicKey: pbk, ShortID: sid, Flow: q.Get("flow"),
		Fingerprint: q.Get("fp"),
	}, nil
}
