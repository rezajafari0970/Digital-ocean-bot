package digitalocean

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
)

func (c *Client) listAll(ctx context.Context, path, field string, target any) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Slice {
		return ErrProviderRequest
	}
	next := path
	seen := map[string]bool{}
	for next != "" {
		if seen[next] || len(seen) >= 1000 {
			return fmt.Errorf("%w: pagination cycle or limit", ErrProviderRequest)
		}
		seen[next] = true
		var raw map[string]json.RawMessage
		if err := c.get(ctx, next, &raw); err != nil {
			return err
		}
		if b, ok := raw[field]; !ok || len(b) == 0 || string(b) == "null" {
			return fmt.Errorf("%w: missing list inventory", ErrProviderRequest)
		}
		if len(raw["links"]) > 0 {
			var checked struct{ Pages struct{ Next string } }
			if json.Unmarshal(raw["links"], &checked) != nil {
				return fmt.Errorf("%w: invalid pagination", ErrProviderRequest)
			}
			if checked.Pages.Next != "" && extractNext(raw["links"]) == "" {
				return fmt.Errorf("%w: invalid next page", ErrProviderRequest)
			}
		}
		part := reflect.New(rv.Elem().Type()).Interface()
		if err := json.Unmarshal(raw[field], part); err != nil {
			return fmt.Errorf("%w: list decode", ErrProviderRequest)
		}
		rv.Elem().Set(reflect.AppendSlice(rv.Elem(), reflect.ValueOf(part).Elem()))
		next = extractNext(raw["links"])
	}
	return nil
}
func extractNext(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var links struct{ Pages struct{ Next string } }
	if json.Unmarshal(raw, &links) != nil || links.Pages.Next == "" {
		return ""
	}
	u, err := url.Parse(links.Pages.Next)
	if err != nil {
		return ""
	}
	p := strings.TrimPrefix(u.Path, "/v2/")
	p = strings.TrimPrefix(p, "/")
	if u.RawQuery != "" {
		return p + "?" + u.RawQuery
	}
	return p
}
