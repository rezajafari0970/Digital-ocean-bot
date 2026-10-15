package transportapply

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/sanaei/realityconfig"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
	"sort"
	"strings"
)

// VerifyRuntime validates the generated config and running binary after a native write.
func VerifyRuntime(ctx context.Context, ssh RemoteSSH, target provisioning.Target, key []byte, payload realityconfig.Payload, native []byte, previousProcess string) error {
	m, e := decode(native)
	if e != nil {
		return e
	}
	settings, e := object(m["settings"])
	if e != nil {
		return e
	}
	cs, e := clients(settings)
	if e != nil {
		return e
	}
	disabled := map[string]bool{}
	if stats, ok := m["clientStats"].([]any); ok {
		for _, v := range stats {
			if x, ok := v.(map[string]any); ok && x["enable"] == false {
				disabled[stringField(x, "email")] = true
			}
		}
	}
	ids := []string{}
	for _, v := range cs {
		c := v.(map[string]any)
		if c["enable"] == true && !disabled[stringField(c, "email")] {
			ids = append(ids, digest(stringField(c, "id")+"\x00"+stringField(c, "email")))
		}
	}
	sort.Strings(ids)
	stream := payload.StreamSettings
	spec := map[string]any{"previous_process": previousProcess, "clients": ids, "port": payload.Port, "network": stream["network"], "security": stream["security"], "flow": payload.Settings["flow"], "decryption_hash": digest(stringField(payload.Settings, "decryption")), "xhttp": stream["xhttpSettings"], "finalmask": stream["finalmask"]}
	if reality, ok := stream["realitySettings"].(map[string]any); ok {
		spec["private_hash"] = digest(stringField(reality, "privateKey"))
		spec["shortIds"] = reality["shortIds"]
		spec["serverNames"] = reality["serverNames"]
	}
	data, e := json.Marshal(spec)
	if e != nil {
		return e
	}
	cmd := "python3 - <<'DOB_VERIFY_TRANSPORT'\nimport base64,json,hashlib,pathlib,subprocess,os,time\nwant=json.loads(base64.b64decode('" + base64.StdEncoding.EncodeToString(data) + "'))\n" + runtimeVerifier + "\nDOB_VERIFY_TRANSPORT"
	out, e := ssh.Run(ctx, target, key, cmd)
	if e != nil || !strings.Contains(out, "\"verified\":true") {
		return errors.New("running core transport not verified")
	}
	return nil
}

//go:embed runtime_verify.py
var runtimeVerifier string

func RuntimeProcessID(ctx context.Context, ssh RemoteSSH, target provisioning.Target, key []byte) (string, error) {
	out, e := ssh.Run(ctx, target, key, `python3 - <<'DOB_RUNTIME_PROCESS'
import pathlib,os,json
core='/usr/local/x-ui/bin/xray-linux-amd64';ids=[];boot=pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()
for p in pathlib.Path('/proc').iterdir():
 if not p.name.isdigit():continue
 try:
  if os.readlink(p/'exe').removesuffix(' (deleted)')!=core:continue
  ticks=(p/'stat').read_text().rsplit(') ',1)[1].split()[19]
  ids.append(p.name+':'+ticks+':'+boot)
 except (OSError,IndexError):pass
if len(ids)>1:raise SystemExit(1)
print(json.dumps({'process':ids[0] if ids else 'none'}))
DOB_RUNTIME_PROCESS`)
	var result struct{ Process string }
	if e != nil || json.Unmarshal([]byte(out), &result) != nil || result.Process == "" {
		return "", errors.New("cannot establish pre-restart process identity")
	}
	return result.Process, nil
}
