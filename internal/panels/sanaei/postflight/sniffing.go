package postflight

import (
	"encoding/json"
	"fmt"
	"sort"
)

func validateSniffing(actualValue any, expected map[string]any) error {
	if expected == nil {
		return nil
	}
	actual, err := object(actualValue)
	if err != nil {
		return fmt.Errorf("%w: sniffing", ErrMismatch)
	}
	for _, key := range []string{"enabled", "metadataOnly", "routeOnly"} {
		if fmt.Sprint(actual[key]) != fmt.Sprint(expected[key]) {
			return fmt.Errorf("%w: sniffing %s", ErrMismatch, key)
		}
	}
	var got, want []string
	gb, _ := json.Marshal(actual["destOverride"])
	wb, _ := json.Marshal(expected["destOverride"])
	if json.Unmarshal(gb, &got) != nil || json.Unmarshal(wb, &want) != nil {
		return fmt.Errorf("%w: sniffing overrides", ErrMismatch)
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		return fmt.Errorf("%w: sniffing overrides", ErrMismatch)
	}
	for i := range got {
		if got[i] != want[i] {
			return fmt.Errorf("%w: sniffing overrides", ErrMismatch)
		}
	}
	return nil
}
