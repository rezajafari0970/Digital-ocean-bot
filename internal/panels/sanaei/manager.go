package sanaei

import (
	"context"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"os"
)

var ErrTemplateMismatch = errors.New("database template checksum mismatch")

type SecretReader interface {
	Get(context.Context, string, string) ([]byte, error)
}
type Runner interface {
	Run(context.Context, provisioning.Target, []byte, string) (string, error)
}
type Uploader interface {
	Upload(context.Context, provisioning.Target, []byte, string, string, os.FileMode) error
}
type DatabaseManager struct {
	Secrets  SecretReader
	Runner   Runner
	Uploader Uploader
}

func (m DatabaseManager) Import(ctx context.Context, target provisioning.Target, t DatabaseTemplate, paths DatabasePaths) error {
	checked, err := InspectTemplate(t.Path)
	if err != nil {
		return err
	}
	if t.SHA256 != "" && checked.SHA256 != t.SHA256 {
		return ErrTemplateMismatch
	}
	key, err := m.Secrets.Get(ctx, target.AccountID, target.KeySecretRef)
	if err != nil {
		return err
	}
	defer wipe(key)
	if err := m.Uploader.Upload(ctx, target, key, t.Path, paths.Incoming, 0600); err != nil {
		return err
	}
	_, err = m.Runner.Run(ctx, target, key, ImportCommand(paths, checked.SHA256))
	return err
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
