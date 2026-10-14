package linode

import (
	"context"
	"net/http"
	"testing"
)

func TestIPInventoryDistinguishesSLAACAndRouted(t *testing.T) {
	d := driver(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" {
			t.Fatal("mutation")
		}
		switch r.URL.Path {
		case "/v4/networking/ips":
			return response(200, page(`[{"address":"203.0.113.1","type":"ipv4","public":true,"linode_id":7,"region":"us-east"},{"address":"2001:db8::1","type":"ipv6","public":true,"linode_id":7,"region":"us-east"}]`, 1, 1, 2)), nil
		case "/v4/networking/ipv6/ranges":
			return response(200, page(`[{"range":"2001:db8:1::","prefix":56,"linodes":[7],"region":"us-east"}]`, 1, 1, 1)), nil
		}
		t.Fatal(r.URL.Path)
		return nil, nil
	})
	o, e := d.ObserveIPs(context.Background())
	if e != nil || len(o.Resources) != 3 || o.Resources[1].Kind != "slaac" || o.Resources[2].Kind != "routed" || o.Resources[2].Prefix != "2001:db8:1::/56" || o.Quotas[0].Limit != nil {
		t.Fatal(o, e)
	}
}
