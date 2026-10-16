package transportcore

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

//go:embed runtime.py
var installer string

//go:embed manifest.json
var manifest json.RawMessage

var ErrPreflight = errors.New("transport core/panel preflight failed; runtime not changed or restored; inspect transport runtime receipt")

type remoteSSH interface {
	Upload(context.Context, provisioning.Target, []byte, string, string, os.FileMode) error
	Run(context.Context, provisioning.Target, []byte, string) (string, error)
}

// RequiresPreparation keeps the Fragment preflight and also prepares managed
// REALITY TCP listeners with bounded unauthenticated fallback forwarding.
func RequiresPreparation(payload realityconfig.Payload) bool {
	network := payload.StreamSettings["network"]
	return network == "xhttp" || ((network == "tcp" || network == "raw") && payload.StreamSettings["security"] == "reality")
}

func Prepare(ctx context.Context, ssh remoteSSH, target provisioning.Target, key []byte, payload realityconfig.Payload) error {
	if !RequiresPreparation(payload) {
		return nil
	}
	data, err := json.Marshal(map[string]any{"log": map[string]any{"loglevel": "none"}, "inbounds": []any{payload}, "outbounds": []any{map[string]any{"protocol": "freedom"}}})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp("", "dob-transport-check-*.json")
	if err != nil {
		return err
	}
	local := f.Name()
	defer os.Remove(local)
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	remote := "/root/.dob-transport-check-" + hex.EncodeToString(nonce) + ".json"
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = ssh.Run(cleanup, target, key, "rm -f -- '"+remote+"'")
	}()
	if err = ssh.Upload(ctx, target, key, local, remote, 0600); err != nil {
		return err
	}

	// The remote validator derives installation requirements from the uploaded
	// payload, so a stale pin cannot bypass the runtime capability check.
	spec, err := json.Marshal(map[string]any{"candidate": remote, "manifest": manifest})
	if err != nil {
		return err
	}
	command := "python3 - <<'DOB_TRANSPORT_RUNTIME'\nimport base64,json\nSPEC=json.loads(base64.b64decode('" + base64.StdEncoding.EncodeToString(spec) + "'))\n" + installer + "\nDOB_TRANSPORT_RUNTIME"
	out, err := ssh.Run(ctx, target, key, command)
	if err != nil || !strings.Contains(out, `"validated":true`) {
		return ErrPreflight
	}
	return nil
}
