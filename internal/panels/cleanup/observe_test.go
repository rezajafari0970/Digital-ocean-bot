package cleanup

import "testing"

func TestSavedScopeRejectsNewChangedAndResurrectedIdentities(t *testing.T) {
	cs := []clientPlan{{Email: "a", ID: "one"}}
	ins := []inboundPlan{{ID: 1, Identity: "known"}}
	for _, o := range []Observed{
		{Clients: map[string]string{"a": "changed"}, Inbounds: map[int64]string{1: "known"}},
		{Clients: map[string]string{"a": "one", "new": "two"}, Inbounds: map[int64]string{1: "known"}},
		{Clients: map[string]string{"a": "one"}, Inbounds: map[int64]string{1: "changed"}},
		{Clients: map[string]string{"a": "one"}, Inbounds: map[int64]string{1: "known", 2: "new"}},
	} {
		if err := validateObserved(o, cs, ins); err == nil {
			t.Fatal("scope change accepted", o)
		}
	}
	cs[0].Absent = true
	if err := validateObserved(Observed{Clients: map[string]string{"a": "one"}, Inbounds: map[int64]string{1: "known"}}, cs, ins); err == nil {
		t.Fatal("resurrection accepted")
	}
}
