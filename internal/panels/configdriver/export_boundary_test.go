package configdriver_test

import (
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/configdriver"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/export"
	"testing"
)

func TestEveryPublicDriverSurvivesOutputFiltering(t *testing.T) {
	r := configdriver.Builtins()
	c, e := configdriver.NewCredentials("profile", "panel", []string{"DIRECT"}, 1)
	if e != nil {
		t.Fatal(e)
	}
	for _, cap := range r.Catalog() {
		if !cap.Public {
			continue
		}
		t.Run(cap.ID, func(t *testing.T) {
			s := configdriver.Spec{Protocol: cap.Protocol, Network: cap.Network, Security: cap.Security, Port: 19443}
			if s.Network != "tcp" {
				s.Host = "transport.example.com"
				s.Path = "/profile"
				if s.Network == "grpc" {
					s.Path = "profile"
				}
			}
			if s.Security == "reality" {
				s.SNI = "example.com"
			}
			uri, e := r.URI(s, "192.0.2.1", "🇩🇪 Germany", c, c.Clients[0])
			if e != nil {
				t.Fatal(e)
			}
			filtered, e := export.ApplyOutputOptions(uri, export.OutputOptions{})
			if e != nil || filtered != uri {
				t.Fatal("public driver dropped or changed by output", e)
			}
		})
	}
}
