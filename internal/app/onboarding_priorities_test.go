package app

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/proxyselection"
	"reflect"
	"testing"
)

func TestOnboardingPriorityOrderAndUncertainty(t *testing.T) {
	var calls []string
	open := func(_ context.Context, id, country string) (*OnboardingRoute, error) {
		calls = append(calls, country+"/"+id)
		if country == "DE" && id == "b" {
			return &OnboardingRoute{Country: country, ProxyID: id, Close: func() {}}, nil
		}
		return nil, unusableOnboarding("wrong country")
	}
	r, e := selectOnboarding(context.Background(), []string{"a", "b"}, []string{"US", "DE", "FR"}, false, open)
	if e != nil || r.Country != "DE" || !reflect.DeepEqual(calls, []string{"US/a", "US/b", "DE/a", "DE/b"}) {
		t.Fatal(r, e, calls)
	}
	calls = nil
	unknown := func(_ context.Context, id, country string) (*OnboardingRoute, error) {
		calls = append(calls, country+"/"+id)
		if id == "a" {
			return nil, errors.New("observation timed out")
		}
		return nil, unusableOnboarding("wrong country")
	}
	if _, e = selectOnboarding(context.Background(), []string{"a", "b"}, []string{"US", "DE"}, false, unknown); e == nil || !reflect.DeepEqual(calls, []string{"US/a", "US/b"}) {
		t.Fatal("unknown allowed descent", calls, e)
	}
	calls = nil
	if _, e = selectOnboarding(context.Background(), nil, []string{proxyselection.Auto}, true, open); e == nil || len(calls) > 0 {
		t.Fatal("browser direct fallback")
	}
	if _, e = selectOnboarding(context.Background(), nil, []string{"US"}, false, open); e == nil {
		t.Fatal("explicit direct fallback")
	}
}
