package main

import "net/url"

// Independent representative hosts from pinned upstream geodata. Synthetic
// native routeTest does not request ads, videos, analytics or push messages.
var expandedCategoryProofHosts = []string{
	"ads.yahoo.com", "adnxs.com", "ad.xiaomi.com", "unityads.unity3d.com",
	"dt.dbankcloud.ru", "insights-collector.gog.com", "firebase.google.com",
	"mtalk.google.com", "play.google.com", "www.youtube.com", "r1.googlevideo.com", "googlevideo.com",
	"lh1.googleusercontent.com", "lh2.googleusercontent.com", "lh3.googleusercontent.com", "lh4.googleusercontent.com",
	"fonts.googleapis.com",
	"scontent.xx.fbcdn.net", "static.xx.fbcdn.net", "video.xx.fbcdn.net",
	"tpc.googlesyndication.com", "googleads.g.doubleclick.net", "play-lh.googleusercontent.com", "s0.2mdn.net",
	"new.googleusercontent.com", "new.fbcdn.net",
}

func verifyExpandedCategories(check func(url.Values, string) error, tag string, emails map[string]string, direct, blocked, protected, udpDeny string, pool map[string][]string, strict bool, deny string) (int, error) {
	count := 0
	for _, network := range []string{"tcp", "udp"} {
		for _, host := range expandedCategoryProofHosts {
			for _, class := range []string{"DIRECT", "RESIDENTIAL"} {
				expected := direct
				if class == "RESIDENTIAL" {
					expected = protectedForNetwork(network, protected, udpDeny)
				}
				form := url.Values{"inboundTag": {tag}, "email": {emails[class]}, "network": {network}, "domain": {host}, "port": {"443"}}
				if err := check(form, expected); err != nil {
					return count, err
				}
				count++
			}
			for _, kind := range []string{"fast", "all"} {
				key := "@positive:" + network + ":" + kind
				if len(pool[key]) == 0 {
					continue
				}
				form := url.Values{"inboundTag": {"dob-route-pool-in-" + network + "-" + kind}, "network": {network}, "domain": {host}, "port": {"443"}}
				if err := check(form, key); err != nil {
					return count, err
				}
				count++
			}
		}
		for _, host := range []string{"huawei.com", "gog.com", "firebase.google.com.evil.test", "insights-collector.gog.com.evil.test", "www.youtube.com.evil.test", "googleusercontent.com.evil.test", "notgoogleusercontent.com", "dns.google.evil.test", "fonts.gstatic.com", "fundingchoicesmessages.google.com", "graph.facebook.com", "dns.google"} {
			form := url.Values{"inboundTag": {tag}, "email": {emails["RESIDENTIAL"]}, "network": {network}, "domain": {host}, "port": {"443"}}
			if err := check(form, blocked); err != nil {
				return count, err
			}
			count++
		}
	}
	// Positive membership must not accidentally be restricted to HTTPS ports.
	for _, class := range []string{"DIRECT", "RESIDENTIAL"} {
		expected := direct
		if class == "RESIDENTIAL" {
			expected = protected
		}
		form := url.Values{"inboundTag": {tag}, "email": {emails[class]}, "network": {"tcp"}, "domain": {"www.youtube.com"}, "port": {"80"}}
		if err := check(form, expected); err != nil {
			return count, err
		}
		count++
	}
	// HTTP UDP denial is unrestricted by destination port; inner DNS guards
	// must win even for an otherwise allowed legacy or compatibility host.
	if udpDeny != "" {
		form := url.Values{"inboundTag": {tag}, "email": {emails["RESIDENTIAL"]}, "network": {"udp"}, "domain": {"www.youtube.com"}, "port": {"8443"}}
		if err := check(form, deny); err != nil {
			return count, err
		}
		count++
	}
	if strict {
		for _, network := range []string{"tcp", "udp"} {
			for _, kind := range []string{"fast", "all"} {
				if len(pool["@inner:"+network+":"+kind]) == 0 {
					continue
				}
				for _, host := range []string{"adservice.google.com", "dt.dbankcloud.ru"} {
					form := url.Values{"inboundTag": {"dob-route-pool-in-" + network + "-" + kind}, "network": {network}, "domain": {host}, "port": {"53"}}
					if err := check(form, deny); err != nil {
						return count, err
					}
					count++
				}
			}
		}
	}
	return count, nil
}
