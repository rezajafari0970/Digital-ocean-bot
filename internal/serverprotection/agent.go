package serverprotection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

func atomicJSON(path string, value any) error {
	b, e := json.Marshal(value)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".guardian-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(append(b, '\n'))
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Rename(tmp, path); e != nil {
		return e
	}
	dir, e := os.Open(filepath.Dir(path))
	if e == nil {
		defer dir.Close()
		e = dir.Sync()
	}
	return e
}
func readPolicy(path string) (Policy, error) {
	var p Policy
	b, e := os.ReadFile(path)
	if e != nil {
		return p, e
	}
	if len(b) > 8192 {
		return p, errors.New("oversized policy")
	}
	e = json.Unmarshal(b, &p)
	if e == nil {
		e = p.Validate()
	}
	return p, e
}
func lock(path string) (*os.File, error) {
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("guardian operation lock: %w", e)
	}
	return f, nil
}
func Configure(r io.Reader) error {
	var p Policy
	d := json.NewDecoder(io.LimitReader(r, 8192))
	d.DisallowUnknownFields()
	if e := d.Decode(&p); e != nil {
		return e
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return errors.New("trailing policy data")
	}
	if e := p.Validate(); e != nil {
		return e
	}
	f, e := lock(RuntimeDir + "/policy.lock")
	if e != nil {
		return e
	}
	defer f.Close()
	old, e := readPolicy(PolicyPath)
	if e == nil {
		if p.PanelID != old.PanelID || p.Revision < old.Revision {
			return errors.New("stale or foreign policy")
		}
		if p.Revision == old.Revision {
			a, _ := json.Marshal(p)
			b, _ := json.Marshal(old)
			if string(a) != string(b) {
				return errors.New("conflicting policy revision")
			}
			return nil
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	return atomicJSON(PolicyPath, p)
}

type statusFile struct {
	Status
	Uptime float64 `json:"sample_uptime"`
	BootID string  `json:"boot_id"`
}

func ReadStatus() (Status, error) {
	var data statusFile
	b, e := os.ReadFile(RuntimeDir + "/status.json")
	if e != nil {
		return data.Status, e
	}
	if e = json.Unmarshal(b, &data); e != nil {
		return data.Status, e
	}
	s := data.Status
	up, boot, e := uptime()
	if e != nil {
		return s, e
	}
	if boot != data.BootID || up < data.Uptime {
		s.SampleAgeMS = 1 << 40
	} else {
		s.SampleAgeMS = int64((up - data.Uptime) * 1000)
	}
	f, e := lock(RuntimeDir + "/agent.lock")
	if e == nil {
		f.Close()
		s.AgentRunning = false
	} else if errors.Is(e, syscall.EWOULDBLOCK) {
		s.AgentRunning = true
	} else {
		return s, e
	}
	return s, nil
}
func uptime() (float64, string, error) {
	b, e := os.ReadFile("/proc/uptime")
	if e != nil {
		return 0, "", e
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0, "", errors.New("uptime missing")
	}
	u, e := strconv.ParseFloat(f[0], 64)
	if e != nil {
		return 0, "", e
	}
	b, e = os.ReadFile("/proc/sys/kernel/random/boot_id")
	return u, strings.TrimSpace(string(b)), e
}
func saveStatus(s Status) error {
	u, b, e := uptime()
	if e != nil {
		return e
	}
	return atomicJSON(RuntimeDir+"/status.json", statusFile{Status: s, Uptime: u - float64(s.SampleAgeMS)/1000, BootID: b})
}
func nftBinary() (NFT, error) { p, e := exec.LookPath("nft"); return NFT{Path: p}, e }
func Cleanup(ctx context.Context) error {
	p, e := readPolicy(PolicyPath)
	if e != nil {
		return e
	}
	if p.Enabled {
		return errors.New("cleanup requires a disabled policy")
	}
	f, e := lock(RuntimeDir + "/agent.lock")
	if e != nil {
		return e
	}
	defer f.Close()
	n, e := nftBinary()
	if e != nil {
		return errors.New("nft unavailable; cannot verify cleanup")
	}
	if e = n.Remove(ctx); e != nil {
		return e
	}
	return saveStatus(Status{Version: Version, Revision: p.Revision, State: "DISABLED", Reason: "owned admission rules removed", ObservedAt: time.Now().UTC(), Ports: []int{}, NFTSupported: true})
}
func RunAgent(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	f, e := lock(RuntimeDir + "/agent.lock")
	if e != nil {
		return e
	}
	defer f.Close()
	p, e := readPolicy(PolicyPath)
	if e != nil {
		return e
	}
	nft, ne := nftBinary()
	sampler := Sampler{}
	engine := Engine{}
	status := Status{Version: Version, Ports: []int{}, XUIState: "unknown"}
	var repairAllowed atomic.Bool
	type repairResult struct{ state, detail string }
	repairs := make(chan repairResult, 1)
	go func() {
		for {
			state, detail := checkRecovery(ctx, repairAllowed.Load())
			select {
			case repairs <- repairResult{state, detail}:
			default:
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
	var appliedRevision int64
	var lastRefresh, lastSave, lastApply, lastAttempt time.Time
	var lastPorts, applyError string
	lastBlock := false
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	defer func() {
		if ne == nil {
			c, done := context.WithTimeout(context.Background(), 2*time.Second)
			defer done()
			_ = nft.Remove(c)
		}
	}()
	for {
		now := time.Now()
		if now.Sub(lastRefresh) >= 2*time.Second {
			candidate, err := readPolicy(PolicyPath)
			if err != nil {
				return err
			}
			if candidate.PanelID != p.PanelID || candidate.Revision < p.Revision {
				return errors.New("policy revision regressed")
			}
			p = candidate
			lastRefresh = now
			if appliedRevision != p.Revision {
				engine = Engine{}
				appliedRevision = p.Revision
				lastApply = time.Time{}
				lastAttempt = time.Time{}
			}
			status.Revision = p.Revision
			status.Enabled = p.Enabled
			status.AgentRunning = true
			if !p.Enabled {
				repairAllowed.Store(false)
				if ne != nil {
					return ne
				}
				if e = nft.Remove(ctx); e != nil {
					return e
				}
				status.State = "DISABLED"
				status.Reason = "owned admission rules removed"
				status.AdmissionBlocked = false
				status.ObservedAt = now.UTC()
				status.AgentRunning = false
				return saveStatus(status)
			}
			status.Ports, e = LivePorts(p)
			if e != nil {
				status.Ports = nil
				status.State = "UNSUPPORTED"
				status.Reason = e.Error()
			}
		}
		m, readErr := sampler.Read(now)
		status.Metrics = m
		status.ObservedAt = now.UTC()
		status.SampleAgeMS = 0
		block := false
		if readErr != nil {
			status.State = "ERROR"
			status.Reason = readErr.Error()
		} else if ne != nil {
			status.State = "UNSUPPORTED"
			status.Reason = "nft binary unavailable"
			status.NFTSupported = false
		} else if len(status.Ports) == 0 {
			status.State = "UNSUPPORTED"
			status.Reason = "no eligible public VLESS TCP ports"
		} else {
			status.State, status.Reason, block = engine.Evaluate(m, now)
		}
		portsKey := fmt.Sprint(status.Ports)
		due := lastApply.IsZero() || lastBlock != block || portsKey != lastPorts || now.Sub(lastApply) >= 5*time.Second
		if ne == nil && due && (lastAttempt.IsZero() || now.Sub(lastAttempt) >= time.Second || lastBlock != block) {
			start := time.Now()
			lastAttempt = now
			if e = nft.Apply(ctx, status.Ports, block); e != nil {
				applyError = e.Error()
				status.State = "ERROR"
				status.Reason = applyError
				status.NFTSupported = false
				status.AdmissionBlocked = lastBlock
			} else {
				applyError = ""
				status.NFTSupported = true
				status.AdmissionBlocked = block
				lastBlock = block
				lastPorts = portsKey
				lastApply = now
			}
			status.ActuationMS = float64(time.Since(start).Microseconds()) / 1000
		}
		if applyError != "" {
			status.State = "ERROR"
			status.Reason = applyError
		}
		repairAllowed.Store(status.State == "READY" && status.NFTSupported && readErr == nil && !block && len(status.Ports) > 0)
		select {
		case result := <-repairs:
			status.XUIState = result.state
			status.Recovery = result.detail
		default:
		}
		if now.Sub(lastSave) >= time.Second {
			status.SampleAgeMS = time.Since(now).Milliseconds()
			if e = saveStatus(status); e != nil {
				return e
			}
			lastSave = now
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}
func checkRecovery(ctx context.Context, allow bool) (string, string) {
	out, e := runCommand(ctx, "systemctl", []string{"show", "x-ui", "--property=ActiveState", "--value"}, "")
	state := strings.TrimSpace(string(out))
	if e != nil {
		return "unknown", "systemd state unavailable"
	}
	if state == "active" || state == "activating" || state == "deactivating" {
		return state, ""
	}
	if !allow {
		return state, "repair withheld until protection is ready and resources recover"
	}
	if state != "failed" && state != "inactive" {
		return state, "no automatic action"
	}
	if _, e = os.Stat("/etc/x-ui/x-ui.db"); e != nil {
		return state, "panel database unavailable; repair withheld"
	}
	f, e := lock("/run/lock/dob-xui-recovery.lock")
	if e != nil {
		return state, "another recovery owner"
	}
	defer f.Close()
	pf, e := lock(RuntimeDir + "/policy.lock")
	if e != nil {
		return state, "policy update in progress"
	}
	defer pf.Close()
	p, e := readPolicy(PolicyPath)
	if e != nil || !p.Enabled {
		return state, "protection disabled"
	}
	var attempts []int64
	b, e := os.ReadFile(StateDir + "/restarts.json")
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return state, "recovery ledger unavailable"
	}
	if e == nil && json.Unmarshal(b, &attempts) != nil {
		return state, "invalid recovery ledger"
	}
	now := time.Now().Unix()
	recent := []int64{}
	for _, a := range attempts {
		if a > now-600 {
			recent = append(recent, a)
		}
	}
	if len(recent) >= 3 || (len(recent) > 0 && now-recent[len(recent)-1] < 60) {
		return state, "recovery cooldown"
	}
	if e = atomicJSON(StateDir+"/restarts.json", append(recent, now)); e != nil {
		return state, "cannot persist recovery intent"
	}
	if _, e = runCommand(ctx, "systemctl", []string{"start", "--no-block", "x-ui"}, ""); e != nil {
		return state, "recovery request failed"
	}
	return state, "inactive service start requested"
}
