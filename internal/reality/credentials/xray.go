package credentials

import (
	"context"
	"errors"
)

var ErrKeyGeneration = errors.New("xray key generation failed")

type RunFunc func(context.Context, string) (string, error)
type XrayGenerator struct {
	Run    RunFunc
	Binary string
}

func (g XrayGenerator) Generate(ctx context.Context) (KeyPair, error) {
	if g.Run == nil {
		return KeyPair{}, ErrKeyGeneration
	}
	bin := g.Binary
	if bin == "" {
		bin = "/usr/local/x-ui/bin/xray"
	}
	out, err := g.Run(ctx, bin+" x25519")
	if err != nil {
		return KeyPair{}, ErrKeyGeneration
	}
	return ParseX25519(out)
}
