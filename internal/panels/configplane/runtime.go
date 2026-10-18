package configplane

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/rezajafari0970/Digital-ocean-bot/internal/panels/transportcore"
	"github.com/rezajafari0970/Digital-ocean-bot/internal/provisioning"
)

//go:embed traffic_probe.py
var trafficProbeScript string

//go:embed runtime.py
var runtimeBody string

var runtimeScript = trafficProbeScript + "\n" + runtimeBody

// The native panel can hot-add inbounds through Xray's API without changing its
// startup file. Verify the running process, its actual socket, active users and
// authenticated traffic, rather than mistaking the startup file for live state.
func (s Service) verifyRuntime(ctx context.Context, t provisioning.Target, key []byte, payload map[string]any) error {
	ssh, ok := s.SSH.(interface {
		RunInput(context.Context, provisioning.Target, []byte, string, []byte) (provisioning.CommandResult, error)
	})
	if !ok {
		return errors.New("profile_runtime_stdin_unavailable")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var obj map[string]any
	if json.Unmarshal(b, &obj) != nil {
		return errors.New("profile_runtime_payload_invalid")
	}
	settings, ok := obj["settings"].(map[string]any)
	if !ok {
		return errors.New("profile_runtime_payload_invalid")
	}
	emails := []string{}
	clients, ok := settings["clients"].([]any)
	if !ok {
		return errors.New("profile_runtime_payload_invalid")
	}
	for _, v := range clients {
		c, ok := v.(map[string]any)
		if !ok {
			return errors.New("profile_runtime_payload_invalid")
		}
		email, ok := c["email"].(string)
		if !ok || email == "" {
			return errors.New("profile_runtime_payload_invalid")
		}
		emails = append(emails, email)
	}
	wanted := map[string]any{"protocol": obj["protocol"], "port": obj["port"], "tag": obj["tag"], "emails": emails, "probes": obj["probe_outbounds"], "probe_classes": obj["probe_classes"], "core_hashes": profileCoreHashes()}
	input, err := json.Marshal(wanted)
	if err != nil {
		return err
	}
	// Only the fixed program is in argv; credentials stay in encrypted SSH stdin.
	command := "python3 -c \"import base64;exec(base64.b64decode('" + base64.StdEncoding.EncodeToString([]byte(runtimeScript)) + "'))\""
	out, err := ssh.RunInput(ctx, t, key, command, input)
	var result struct {
		Verified    bool
		Reason      string
		Classes     map[string]bool
		Diagnostics map[string]trafficDiagnostic `json:"class_diagnostics"`
	}
	decodeErr := json.Unmarshal([]byte(out.Stdout), &result)
	if decodeErr == nil {
		logTrafficFailures("", t.DropletID, result.Classes, result.Diagnostics)
	}
	if err != nil || decodeErr != nil || !result.Verified {
		switch result.Reason {
		case "binary", "socket", "users", "traffic":
			return errors.New("profile_runtime_" + result.Reason + "_unverified")
		}
		return errors.New("profile_runtime_unverified")
	}
	verifiedClasses := []string{}
	for class, valid := range result.Classes {
		if valid {
			verifiedClasses = append(verifiedClasses, class)
		}
	}
	payload["verified_classes"] = verifiedClasses
	// A node-local successful client does not prove its cloud firewall admits
	// the published endpoint. Require reachability from the controller too.
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, e := (&net.Dialer{}).DialContext(probe, "tcp", net.JoinHostPort(t.Host, fmt.Sprint(payload["port"])))
	if e != nil {
		return errors.New("profile_public_port_unreachable")
	}
	_ = conn.Close()
	return nil
}

// Official v26.9.9 assets are pinned alongside the installed compatibility core.
// Their archives were verified against the release .dgst files before deriving
// these executable hashes. Unknown binaries still cannot authorize publication.
func profileCoreHashes() []string {
	return append(transportcore.CoreHashes(),
		// Previous pinned binaries remain recognized only for the rolling
		// transition. Each proof schedules replacement when core_current=false.
		"f0e35cb830cd58756dae9744a0d9372f972dfba1ba6a50a30422ba3e8727eeed",
		"d39f86129ec689fe7791365de08017c0b3ba0acb231467eac6da92f938c530a8",
		"c4ae6798c38e0e5343b192406746333cd0ba7ff3eb984f8c4b9939dcb68c3f8a",
		"c1defe42b6db958a97c5e049a02a00a4baaedaca7b51c1c229f0830e288acef5")
}

type trafficDiagnostic struct {
	Code     string `json:"code"`
	Attempts int    `json:"attempts"`
}

// Diagnostics explain withheld evidence; they never participate in acceptance.
func (d trafficDiagnostic) safe() bool {
	if d.Attempts < 1 || d.Attempts > 2 {
		return false
	}
	switch d.Code {
	case "ok", "empty_body", "tls_certificate", "timeout", "transport":
		return true
	}
	if strings.HasPrefix(d.Code, "http_") && len(d.Code) == 8 {
		n, e := strconv.Atoi(d.Code[5:])
		return e == nil && n >= 100 && n <= 599
	}
	return false
}
func logTrafficFailures(profile, target string, classes map[string]bool, details map[string]trafficDiagnostic) {
	for _, class := range []string{"DIRECT", "RESIDENTIAL"} {
		d, ok := details[class]
		if ok && !classes[class] && d.safe() {
			log.Printf("config traffic proof profile=%s target=%s class=%s code=%s attempts=%d", profile, target, class, d.Code, d.Attempts)
		}
	}
}
