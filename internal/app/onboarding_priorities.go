package app

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/network"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxyselection"
)

// OnboardingRoute owns one disposable identity and its immutable proxy snapshot.
// Close is mandatory. Browser callers require a verified identity even for AUTO.
type OnboardingRoute struct {
	Client                         *http.Client
	Close                          func()
	Check                          func(context.Context) error
	Dial                           func(context.Context, string) (net.Conn, error)
	ProxyID, Country, IP, Timezone string
	StrictIdentity                 bool
}
type onboardingSelectionError struct{ evaluation proxyselection.Evaluation }

func (e *onboardingSelectionError) Error() string {
	return "proxy_country_unverified: " + e.evaluation.Reason
}
func unusableOnboarding(reason string) error {
	return &onboardingSelectionError{proxyselection.Evaluation{Health: proxyselection.Unusable, Reason: reason, ExhaustProvider: true}}
}

func (c Container) OnboardingPriorities(ctx context.Context, ids, countries []string, browser bool) (*OnboardingRoute, error) {
	return selectOnboarding(ctx, ids, countries, browser, func(ctx context.Context, id, country string) (*OnboardingRoute, error) {
		r := &OnboardingRoute{StrictIdentity: browser}
		_, _, _, err := c.onboardingClient(ctx, id, country, func(_ context.Context, identity string, p network.Proxy, credentials network.ProxyCredentials) (*network.Gateway, error) {
			return network.NewProxyGateway(identity, p, credentials)
		}, r)
		return r, err
	})
}

func selectOnboarding(ctx context.Context, ids, countries []string, browser bool, open func(context.Context, string, string) (*OnboardingRoute, error)) (*OnboardingRoute, error) {
	policy, err := proxyselection.Priorities("", countries)
	if err != nil {
		return nil, err
	}
	if len(ids) > 3 {
		return nil, errors.New("at most three assigned proxies")
	}
	if len(ids) == 0 {
		if browser || !proxyselection.Legacy(policy) {
			return nil, errors.New("proxy_country_requires_proxy")
		}
		return open(ctx, "", proxyselection.Auto)
	}
	pool := []proxyselection.Provider{}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			return nil, errors.New("invalid proxy priorities")
		}
		seen[id] = true
		pool = append(pool, proxyselection.Provider{ID: id, Attempts: 6})
	}
	// AUTO may select an eligible proxy, but never a direct connection.
	if proxyselection.Legacy(policy) {
		for _, id := range ids {
			r, e := open(ctx, id, proxyselection.Auto)
			if e == nil {
				return r, nil
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
		return nil, errors.New("no verified eligible proxy")
	}
	state := proxyselection.Checkpoint{}
	var selected *OnboardingRoute
	result := (proxyselection.Selector{}).Select(ctx, "onboarding", policy, pool, &state, func(ctx context.Context, q proxyselection.Request) proxyselection.Evaluation {
		r, e := open(ctx, q.Provider, q.Country)
		if e == nil {
			selected = r
			return proxyselection.Evaluation{Health: proxyselection.Usable}
		}
		var known *onboardingSelectionError
		if errors.As(e, &known) {
			return known.evaluation
		}
		return proxyselection.Evaluation{Health: proxyselection.Unknown, Reason: "route verification unavailable"}
	})
	if result.Kind == "selected" && selected != nil {
		return selected, nil
	}
	if selected != nil {
		selected.Close()
	}
	return nil, errors.New("proxy_country_unverified: " + result.Reason)
}
