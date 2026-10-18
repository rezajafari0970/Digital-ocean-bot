package transportcore

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"time"
)

// PrepareCurrent validates the actual native configuration and installs the
// pinned runtime with the same identity journal and rollback as Prepare.
// The caller must hold the panel's configuration writer lease.
func PrepareCurrent(ctx context.Context, ssh interface {
	Run(context.Context, provisioning.Target, []byte, string) (string, error)
}, target provisioning.Target, key []byte) error {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	candidate := "/root/.dob-transport-check-" + hex.EncodeToString(nonce) + ".json"
	spec, err := json.Marshal(map[string]any{"candidate": candidate, "manifest": manifest, "force_core": true})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_, _ = ssh.Run(cleanup, target, key, "rm -f -- '"+candidate+"'")
	}()
	program := `import base64,json,os,shutil
SPEC=json.loads(base64.b64decode('` + base64.StdEncoding.EncodeToString(spec) + `'))
os.umask(0o077)
fd=os.open(SPEC['candidate'],os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
with os.fdopen(fd,'wb') as target,open('/usr/local/x-ui/bin/config.json','rb') as source:shutil.copyfileobj(source,target)
try:
 exec(base64.b64decode('` + base64.StdEncoding.EncodeToString([]byte(installer)) + `'))
finally:
 os.unlink(SPEC['candidate'])
`
	out, err := ssh.Run(ctx, target, key, "python3 - <<'DOB_CURRENT_TRANSPORT_RUNTIME'\n"+program+"\nDOB_CURRENT_TRANSPORT_RUNTIME")
	var receipt struct {
		Validated bool
		Installed bool
	}
	if err != nil || json.Unmarshal([]byte(out), &receipt) != nil || !receipt.Validated || !receipt.Installed {
		return ErrPreflight
	}
	return nil
}
