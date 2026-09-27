package runtimecap

import (
	"context"
	"errors"
	"strings"
)

var ErrXrayNotFound = errors.New("xray runtime not found")

type RunFunc func(context.Context, string) (string, error)
type XrayResolver struct{ Run RunFunc }

func (r XrayResolver) Resolve(ctx context.Context) (string, error) {
	if r.Run == nil {
		return "", ErrXrayNotFound
	}
	const command = `set +e
for p in "$(command -v xray 2>/dev/null)" /usr/local/x-ui/bin/xray /usr/local/x-ui/bin/xray-linux-amd64 /usr/local/x-ui/bin/xray-linux-64 /usr/local/bin/xray /usr/bin/xray; do
 [ -n "$p" ] || continue
 [ -x "$p" ] || continue
 "$p" version >/dev/null 2>&1 || continue
 printf '%s\n' "$p"
 exit 0
done
exit 0`
	out, err := r.Run(ctx, command)
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(out)
	if path == "" || strings.ContainsAny(path, "\r\n\x00") {
		return "", ErrXrayNotFound
	}
	return path, nil
}
